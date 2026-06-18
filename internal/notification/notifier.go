/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package notification implements alerting (Slack, Email, Webhook) for ClamAV
// scan results with exponential-backoff retry. It is intentionally decoupled
// from the reconciler so that it can be unit-tested without a live cluster.
package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

const (
	// maxRetries is the number of delivery attempts per channel before giving up.
	maxRetries = 3
	// retryBaseDelay is the initial backoff delay; doubled on each subsequent attempt.
	retryBaseDelay = 5 * time.Second
)

// NotifyResult carries the per-channel outcome of a SendWithRetry call.
// It lets the caller (controller) decide what to record in the NodeScan status
// without coupling the notifier to the API types.
type NotifyResult struct {
	Channel string
	Err     error
}

// Notifier dispatches scan-result alerts to configured channels.
// It holds its own client and recorder so it can be used independently
// of any specific reconciler struct.
type Notifier struct {
	Client   client.Client
	Recorder record.EventRecorder
}

// New returns a ready-to-use Notifier.
func New(c client.Client, recorder record.EventRecorder) *Notifier {
	return &Notifier{Client: c, Recorder: recorder}
}

// SendWithRetry dispatches all configured notifications for the given NodeScan.
// Each channel (Slack, Email, Webhook) is retried up to maxRetries times with
// exponential backoff before being declared failed.
//
// Results (one per enabled channel) are returned so the caller can record
// delivery failures in the NodeScan status or emit Prometheus metrics.
// The context should carry a generous deadline (e.g. 5 min) to allow retries.
func (n *Notifier) SendWithRetry(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) []NotifyResult {
	logger := log.FromContext(ctx)

	if scanPolicy.Spec.Notifications == nil {
		return nil
	}

	var results []NotifyResult

	type channelFn struct {
		name string
		fn   func(context.Context, *clamavv1alpha1.NodeScan, *clamavv1alpha1.ScanPolicy) error
	}

	var channels []channelFn
	if slack := scanPolicy.Spec.Notifications.Slack; slack != nil && slack.Enabled {
		channels = append(channels, channelFn{"slack", n.sendSlack})
	}
	if email := scanPolicy.Spec.Notifications.Email; email != nil && email.Enabled {
		channels = append(channels, channelFn{"email", n.sendEmail})
	}
	if wh := scanPolicy.Spec.Notifications.Webhook; wh != nil && wh.Enabled {
		channels = append(channels, channelFn{"webhook", n.sendWebhook})
	}
	if teams := scanPolicy.Spec.Notifications.Teams; teams != nil && teams.Enabled {
		channels = append(channels, channelFn{"teams", n.sendTeams})
	}

	for _, ch := range channels {
		var lastErr error
		delay := retryBaseDelay

		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := ch.fn(ctx, nodeScan, scanPolicy); err == nil {
				lastErr = nil
				logger.V(1).Info("Notification delivered",
					"channel", ch.name, "attempt", attempt)
				break
			} else {
				lastErr = err
				logger.Error(err, "Notification attempt failed — will retry",
					"channel", ch.name, "attempt", attempt, "maxRetries", maxRetries)

				if attempt < maxRetries {
					select {
					case <-ctx.Done():
						lastErr = fmt.Errorf("context canceled during retry: %w", ctx.Err())
						goto done
					case <-time.After(delay):
					}
					delay *= 2
				}
			}
		}
	done:
		result := NotifyResult{Channel: ch.name, Err: lastErr}
		if lastErr != nil {
			logger.Error(lastErr, "Notification delivery failed after all retries",
				"channel", ch.name, "attempts", maxRetries)
			n.Recorder.Event(nodeScan, corev1.EventTypeWarning, "NotificationFailed",
				fmt.Sprintf("Failed to send %s notification after %d attempts: %v",
					ch.name, maxRetries, lastErr))
		}
		results = append(results, result)
	}

	return results
}

// ─── Slack ────────────────────────────────────────────────────────────────────

func (n *Notifier) sendSlack(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) error {
	config := scanPolicy.Spec.Notifications.Slack

	// OnlyOnInfection is *bool; nil means unset → the CRD default (true) applies.
	if ptr.Deref(config.OnlyOnInfection, true) && nodeScan.Status.FilesInfected == 0 {
		return nil
	}

	webhookURL := config.WebhookURL
	if config.WebhookSecretRef != nil {
		secret := &corev1.Secret{}
		if err := n.Client.Get(ctx, types.NamespacedName{
			Name:      config.WebhookSecretRef.Name,
			Namespace: scanPolicy.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get webhook secret: %w", err)
		}
		webhookURL = string(secret.Data[config.WebhookSecretRef.Key])
	}

	if webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	color, icon := "good", "✅"
	if nodeScan.Status.FilesInfected > 0 {
		color, icon = "danger", "🚨"
	}

	fields := []map[string]interface{}{
		{"title": "Node", "value": nodeScan.Spec.NodeName, "short": true},
		{"title": "Status", "value": string(nodeScan.Status.Phase), "short": true},
		{"title": "Files Scanned", "value": fmt.Sprintf("%d", nodeScan.Status.FilesScanned), "short": true},
		{"title": "Files Infected", "value": fmt.Sprintf("%d", nodeScan.Status.FilesInfected), "short": true},
		{"title": "Duration", "value": fmt.Sprintf("%d seconds", nodeScan.Status.Duration), "short": true},
	}

	if nodeScan.Status.FilesInfected > 0 {
		var lines []string
		for i, f := range nodeScan.Status.InfectedFiles {
			if i >= 10 {
				lines = append(lines, fmt.Sprintf("... and %d more", len(nodeScan.Status.InfectedFiles)-10))
				break
			}
			lines = append(lines, fmt.Sprintf("• `%s` - %s", f.Path, strings.Join(f.Viruses, ", ")))
		}
		fields = append(fields, map[string]interface{}{
			"title": "Infected Files",
			"value": strings.Join(lines, "\n"),
			"short": false,
		})
	}

	message := map[string]interface{}{
		"channel":    config.Channel,
		"username":   "ClamAV Operator",
		"icon_emoji": ":shield:",
		"text":       fmt.Sprintf("%s ClamAV Scan Completed", icon),
		"attachments": []map[string]interface{}{
			{"color": color, "fields": fields, "footer": "ClamAV Operator", "ts": time.Now().Unix()},
		},
	}

	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to send Slack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack API returned status %d", resp.StatusCode)
	}
	return nil
}

// ─── Email ────────────────────────────────────────────────────────────────────

func (n *Notifier) sendEmail(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) error {
	config := scanPolicy.Spec.Notifications.Email

	// OnlyOnInfection is *bool; nil means unset → the CRD default (true) applies.
	if ptr.Deref(config.OnlyOnInfection, true) && nodeScan.Status.FilesInfected == 0 {
		return nil
	}

	var username, password string
	if config.SMTPAuthSecretRef != nil {
		secret := &corev1.Secret{}
		if err := n.Client.Get(ctx, types.NamespacedName{
			Name:      config.SMTPAuthSecretRef.Name,
			Namespace: scanPolicy.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get SMTP secret: %w", err)
		}
		username = string(secret.Data["username"])
		password = string(secret.Data["password"])
	}

	subject := "ClamAV Scan Completed"
	if nodeScan.Status.FilesInfected > 0 {
		subject = "🚨 ALERT: Malware Detected by ClamAV"
	}

	var buf strings.Builder
	buf.WriteString("================================================================================\n")
	buf.WriteString("                         ClamAV SCAN REPORT\n")
	buf.WriteString("================================================================================\n\n")
	fmt.Fprintf(&buf, "Node:              %s\n", nodeScan.Spec.NodeName)
	fmt.Fprintf(&buf, "Scan Name:         %s\n", nodeScan.Name)
	fmt.Fprintf(&buf, "Status:            %s\n", nodeScan.Status.Phase)
	scanDate := "unknown"
	if nodeScan.Status.StartTime != nil {
		scanDate = nodeScan.Status.StartTime.Format(time.RFC3339)
	}
	fmt.Fprintf(&buf, "Scan Date:         %s\n", scanDate)
	fmt.Fprintf(&buf, "Duration:          %d seconds\n\n", nodeScan.Status.Duration)
	buf.WriteString("STATISTICS:\n")
	buf.WriteString("--------------------------------------------------------------------------------\n")
	fmt.Fprintf(&buf, "Files Scanned:     %d\n", nodeScan.Status.FilesScanned)
	fmt.Fprintf(&buf, "Files Infected:    %d\n", nodeScan.Status.FilesInfected)
	fmt.Fprintf(&buf, "Files Skipped:     %d\n", nodeScan.Status.FilesSkipped)
	fmt.Fprintf(&buf, "Errors:            %d\n\n", nodeScan.Status.ErrorCount)

	if nodeScan.Status.FilesInfected > 0 {
		buf.WriteString("⚠️  INFECTED FILES DETECTED:\n")
		buf.WriteString("================================================================================\n\n")
		for i, f := range nodeScan.Status.InfectedFiles {
			fmt.Fprintf(&buf, "%d. File: %s\n", i+1, f.Path)
			fmt.Fprintf(&buf, "   Viruses: %s\n", strings.Join(f.Viruses, ", "))
			fmt.Fprintf(&buf, "   Size: %d bytes\n\n", f.Size)
		}
	} else {
		buf.WriteString("✅ NO MALWARE DETECTED\n\n")
	}

	buf.WriteString("--------------------------------------------------------------------------------\n")
	buf.WriteString("This is an automated message from ClamAV Operator.\n")
	buf.WriteString("================================================================================\n")

	msg := []byte(
		"From: " + config.From + "\r\n" +
			"To: " + strings.Join(config.Recipients, ",") + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
			buf.String() + "\r\n",
	)

	host := strings.Split(config.SMTPServer, ":")[0]
	auth := smtp.PlainAuth("", username, password, host)

	// TLS is mandatory — no plaintext fallback. If the server does not support
	// TLS, the operator will log a warning and the user must either configure a
	// TLS-capable SMTP server or disable email notifications.
	tlsDialer := &tls.Dialer{Config: &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}} //nolint:gosec
	conn, err := tlsDialer.DialContext(ctx, "tcp", config.SMTPServer)
	if err != nil {
		return fmt.Errorf("failed to establish TLS connection to SMTP server %s: %w "+
			"(plaintext SMTP is not supported for security reasons; "+
			"ensure your SMTP server supports TLS on this port)", config.SMTPServer, err)
	}
	defer conn.Close()

	smtpClient, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	if err = smtpClient.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}
	if err = smtpClient.Mail(config.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	for _, rcpt := range config.Recipients {
		if err = smtpClient.Rcpt(rcpt); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s failed: %w", rcpt, err)
		}
	}
	w, err := smtpClient.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close SMTP writer: %w", err)
	}
	smtpClient.Quit() //nolint:errcheck
	return nil
}

// ─── Webhook ──────────────────────────────────────────────────────────────────

func (n *Notifier) sendWebhook(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) error {
	config := scanPolicy.Spec.Notifications.Webhook

	// OnlyOnInfection is *bool; nil means unset → the CRD default (true) applies.
	if ptr.Deref(config.OnlyOnInfection, true) && nodeScan.Status.FilesInfected == 0 {
		return nil
	}

	payload := map[string]interface{}{
		"type":      "clamav.scan.completed",
		"timestamp": time.Now().Format(time.RFC3339),
		"scan": map[string]interface{}{
			"name":           nodeScan.Name,
			"namespace":      nodeScan.Namespace,
			"node":           nodeScan.Spec.NodeName,
			"phase":          nodeScan.Status.Phase,
			"filesScanned":   nodeScan.Status.FilesScanned,
			"filesInfected":  nodeScan.Status.FilesInfected,
			"filesSkipped":   nodeScan.Status.FilesSkipped,
			"errorCount":     nodeScan.Status.ErrorCount,
			"duration":       nodeScan.Status.Duration,
			"startTime":      nodeScan.Status.StartTime,
			"completionTime": nodeScan.Status.CompletionTime,
		},
		"severity": func() string {
			if nodeScan.Status.FilesInfected > 0 {
				return "critical"
			}
			return "info"
		}(),
	}

	if nodeScan.Status.FilesInfected > 0 {
		var files []map[string]interface{}
		for _, f := range nodeScan.Status.InfectedFiles {
			files = append(files, map[string]interface{}{
				"path": f.Path, "viruses": f.Viruses, "size": f.Size,
			})
		}
		payload["infectedFiles"] = files
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ClamAV-Operator/1.0")
	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	if config.SecretRef != nil {
		secret := &corev1.Secret{}
		if err := n.Client.Get(ctx, types.NamespacedName{
			Name:      config.SecretRef.Name,
			Namespace: scanPolicy.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get webhook secret: %w", err)
		}
		for k, v := range secret.Data {
			req.Header.Set(k, string(v))
		}
	}

	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
	}
	resp, err := httpClient.Do(req) //nolint:gosec // G704: URL comes from operator CRD config, not user input
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// ─── Microsoft Teams ──────────────────────────────────────────────────────────

// sendTeams delivers a notification to a Microsoft Teams channel via Incoming
// Webhook.  The payload uses the Adaptive Card format (schema v1.2) which is
// supported by both the legacy Office 365 Connector webhooks and the new
// Microsoft Workflows webhooks (prod-*.logic.azure.com).
func (n *Notifier) sendTeams(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) error {
	config := scanPolicy.Spec.Notifications.Teams

	// OnlyOnInfection is *bool; nil means unset → the CRD default (true) applies.
	if ptr.Deref(config.OnlyOnInfection, true) && nodeScan.Status.FilesInfected == 0 {
		return nil
	}

	// ── Resolve webhook URL ────────────────────────────────────────────────
	webhookURL := config.WebhookURL
	if config.WebhookSecretRef != nil {
		secret := &corev1.Secret{}
		if err := n.Client.Get(ctx, types.NamespacedName{
			Name:      config.WebhookSecretRef.Name,
			Namespace: scanPolicy.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get Teams webhook secret: %w", err)
		}
		webhookURL = string(secret.Data[config.WebhookSecretRef.Key])
	}
	if webhookURL == "" {
		return fmt.Errorf("teams webhook URL not configured")
	}

	// ── Build Adaptive Card body ───────────────────────────────────────────
	// Status line
	status := "✅ No malware detected"
	themeColor := "00B050" // green
	if nodeScan.Status.FilesInfected > 0 {
		status = fmt.Sprintf("🚨 %d infected file(s) found", nodeScan.Status.FilesInfected)
		themeColor = "FF0000" // red
	}

	// Fact set: always-present fields
	facts := []map[string]string{
		{"title": "Node", "value": nodeScan.Spec.NodeName},
		{"title": "Status", "value": string(nodeScan.Status.Phase)},
		{"title": "Files scanned", "value": fmt.Sprintf("%d", nodeScan.Status.FilesScanned)},
		{"title": "Files infected", "value": fmt.Sprintf("%d", nodeScan.Status.FilesInfected)},
		{"title": "Duration", "value": fmt.Sprintf("%ds", nodeScan.Status.Duration)},
	}
	if nodeScan.Status.ResultsPartial {
		facts = append(facts, map[string]string{
			"title": "⚠️ Warning",
			"value": "Results are partial — re-scan required",
		})
	}

	// Convert facts to Adaptive Card FactSet items
	factItems := make([]map[string]interface{}, len(facts))
	for i, f := range facts {
		factItems[i] = map[string]interface{}{"title": f["title"], "value": f["value"]}
	}

	// Body blocks
	bodyBlocks := []map[string]interface{}{
		{
			"type":   "TextBlock",
			"size":   "Large",
			"weight": "Bolder",
			"color": func() string {
				if nodeScan.Status.FilesInfected > 0 {
					return "Attention"
				}
				return "Good"
			}(),
			"text": fmt.Sprintf("ClamAV Scan — %s", status),
		},
		{
			"type":  "FactSet",
			"facts": factItems,
		},
	}

	// Append infected file list (max 10)
	if nodeScan.Status.FilesInfected > 0 {
		var lines []string
		for i, f := range nodeScan.Status.InfectedFiles {
			if i >= 10 {
				lines = append(lines, fmt.Sprintf("... and %d more", len(nodeScan.Status.InfectedFiles)-10))
				break
			}
			lines = append(lines, fmt.Sprintf("- **%s** → %s", f.Path, strings.Join(f.Viruses, ", ")))
		}
		bodyBlocks = append(bodyBlocks, map[string]interface{}{
			"type":   "TextBlock",
			"weight": "Bolder",
			"text":   "Infected files:",
		}, map[string]interface{}{
			"type": "TextBlock",
			"text": strings.Join(lines, "\n"),
			"wrap": true,
		})
	}

	// Adaptive Card payload — compatible with both legacy connectors and Workflows
	payload := map[string]interface{}{
		"type": "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]interface{}{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.2",
					"body":    bodyBlocks,
					"msteams": map[string]interface{}{
						"width": "Full",
					},
				},
			},
		},
		// Legacy MessageCard fields kept for backward compat with old connectors
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": themeColor,
		"summary":    fmt.Sprintf("ClamAV Scan — %s", status),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Teams payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create Teams request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ClamAV-Operator/1.0")

	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
	}
	resp, err := httpClient.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("teams request failed: %w", err)
	}
	defer resp.Body.Close()

	// Teams webhooks return 200 with body "1" on success, or 400/429 on errors.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("teams webhook returned status %d", resp.StatusCode)
	}
	return nil
}
