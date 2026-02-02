/*
Copyright 2025 Platform Team - Numspot.

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

package controllers

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
	"sigs.k8s.io/controller-runtime/pkg/log"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

// sendNotifications sends notifications about infected files
func (r *NodeScanReconciler) sendNotifications(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) {
	log := log.FromContext(ctx)

	if scanPolicy.Spec.Notifications == nil {
		return
	}

	// Slack
	if scanPolicy.Spec.Notifications.Slack != nil && scanPolicy.Spec.Notifications.Slack.Enabled {
		if err := r.sendSlackNotification(ctx, nodeScan, scanPolicy); err != nil {
			log.Error(err, "failed to send Slack notification")
			r.Recorder.Event(nodeScan, corev1.EventTypeWarning, "NotificationFailed",
				fmt.Sprintf("Failed to send Slack notification: %v", err))
		}
	}

	// Email
	if scanPolicy.Spec.Notifications.Email != nil && scanPolicy.Spec.Notifications.Email.Enabled {
		if err := r.sendEmailNotification(ctx, nodeScan, scanPolicy); err != nil {
			log.Error(err, "failed to send Email notification")
			r.Recorder.Event(nodeScan, corev1.EventTypeWarning, "NotificationFailed",
				fmt.Sprintf("Failed to send Email notification: %v", err))
		}
	}

	// Webhook
	if scanPolicy.Spec.Notifications.Webhook != nil {
		if err := r.sendWebhookNotification(ctx, nodeScan, scanPolicy); err != nil {
			log.Error(err, "failed to send Webhook notification")
			r.Recorder.Event(nodeScan, corev1.EventTypeWarning, "NotificationFailed",
				fmt.Sprintf("Failed to send Webhook notification: %v", err))
		}
	}
}

// sendSlackNotification sends a Slack notification
func (r *NodeScanReconciler) sendSlackNotification(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) error {
	config := scanPolicy.Spec.Notifications.Slack

	// Skip if onlyOnInfection and no infections
	if config.OnlyOnInfection && nodeScan.Status.FilesInfected == 0 {
		return nil
	}

	// Get webhook URL from secret
	webhookURL := config.WebhookURL
	if config.WebhookSecretRef != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
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

	// Build message
	color := "good"
	icon := "✅"
	if nodeScan.Status.FilesInfected > 0 {
		color = "danger"
		icon = "🚨"
	}

	fields := []map[string]interface{}{
		{
			"title": "Node",
			"value": nodeScan.Spec.NodeName,
			"short": true,
		},
		{
			"title": "Status",
			"value": string(nodeScan.Status.Phase),
			"short": true,
		},
		{
			"title": "Files Scanned",
			"value": fmt.Sprintf("%d", nodeScan.Status.FilesScanned),
			"short": true,
		},
		{
			"title": "Files Infected",
			"value": fmt.Sprintf("%d", nodeScan.Status.FilesInfected),
			"short": true,
		},
		{
			"title": "Duration",
			"value": fmt.Sprintf("%d seconds", nodeScan.Status.Duration),
			"short": true,
		},
	}

	// Add infected files details
	if nodeScan.Status.FilesInfected > 0 {
		var infectedList []string
		for i, f := range nodeScan.Status.InfectedFiles {
			if i >= 10 {
				infectedList = append(infectedList, fmt.Sprintf("... and %d more", len(nodeScan.Status.InfectedFiles)-10))
				break
			}
			infectedList = append(infectedList, fmt.Sprintf("• `%s` - %s", f.Path, strings.Join(f.Viruses, ", ")))
		}

		fields = append(fields, map[string]interface{}{
			"title": "Infected Files",
			"value": strings.Join(infectedList, "\n"),
			"short": false,
		})
	}

	message := map[string]interface{}{
		"channel":    config.Channel,
		"username":   "ClamAV Operator",
		"icon_emoji": ":shield:",
		"text":       fmt.Sprintf("%s ClamAV Scan Completed", icon),
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"fields": fields,
				"footer": "ClamAV Operator",
				"ts":     time.Now().Unix(),
			},
		},
	}

	// Send request
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack API returned status %d", resp.StatusCode)
	}

	return nil
}

// sendEmailNotification sends an email notification
func (r *NodeScanReconciler) sendEmailNotification(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) error {
	config := scanPolicy.Spec.Notifications.Email

	// Skip if onlyOnInfection and no infections
	if config.OnlyOnInfection && nodeScan.Status.FilesInfected == 0 {
		return nil
	}

	// Get SMTP credentials from secret
	var username, password string
	if config.SMTPAuthSecretRef != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      config.SMTPAuthSecretRef.Name,
			Namespace: scanPolicy.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get SMTP secret: %w", err)
		}
		username = string(secret.Data["username"])
		password = string(secret.Data["password"])
	}

	// Build email
	subject := "ClamAV Scan Completed"
	if nodeScan.Status.FilesInfected > 0 {
		subject = "🚨 ALERT: Malware Detected by ClamAV"
	}

	var body strings.Builder
	body.WriteString("================================================================================\n")
	body.WriteString("                         ClamAV SCAN REPORT\n")
	body.WriteString("================================================================================\n\n")

	body.WriteString(fmt.Sprintf("Node:              %s\n", nodeScan.Spec.NodeName))
	body.WriteString(fmt.Sprintf("Scan Name:         %s\n", nodeScan.Name))
	body.WriteString(fmt.Sprintf("Status:            %s\n", nodeScan.Status.Phase))
	body.WriteString(fmt.Sprintf("Scan Date:         %s\n", nodeScan.Status.StartTime.Format(time.RFC3339)))
	body.WriteString(fmt.Sprintf("Duration:          %d seconds\n", nodeScan.Status.Duration))
	body.WriteString("\n")

	body.WriteString("STATISTICS:\n")
	body.WriteString("--------------------------------------------------------------------------------\n")
	body.WriteString(fmt.Sprintf("Files Scanned:     %d\n", nodeScan.Status.FilesScanned))
	body.WriteString(fmt.Sprintf("Files Infected:    %d\n", nodeScan.Status.FilesInfected))
	body.WriteString(fmt.Sprintf("Files Skipped:     %d\n", nodeScan.Status.FilesSkipped))
	body.WriteString(fmt.Sprintf("Errors:            %d\n", nodeScan.Status.ErrorCount))
	body.WriteString("\n")

	if nodeScan.Status.FilesInfected > 0 {
		body.WriteString("⚠️  INFECTED FILES DETECTED:\n")
		body.WriteString("================================================================================\n\n")
		for i, f := range nodeScan.Status.InfectedFiles {
			body.WriteString(fmt.Sprintf("%d. File: %s\n", i+1, f.Path))
			body.WriteString(fmt.Sprintf("   Viruses: %s\n", strings.Join(f.Viruses, ", ")))
			body.WriteString(fmt.Sprintf("   Size: %d bytes\n", f.Size))
			body.WriteString("\n")
		}
	} else {
		body.WriteString("✅ NO MALWARE DETECTED\n")
		body.WriteString("\n")
	}

	body.WriteString("--------------------------------------------------------------------------------\n")
	body.WriteString("This is an automated message from ClamAV Operator.\n")
	body.WriteString("For more information, check the Kubernetes cluster logs.\n")
	body.WriteString("================================================================================\n")

	// Compose message
	message := []byte(
		"From: " + config.From + "\r\n" +
			"To: " + strings.Join(config.Recipients, ",") + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			body.String() + "\r\n",
	)

	// Parse server address
	host := strings.Split(config.SMTPServer, ":")[0]

	// Setup TLS
	tlsconfig := &tls.Config{
		ServerName: host,
	}

	// Connect
	auth := smtp.PlainAuth("", username, password, host)

	// Send with TLS
	conn, err := tls.Dial("tcp", config.SMTPServer, tlsconfig)
	if err != nil {
		// Fallback to non-TLS
		err = smtp.SendMail(
			config.SMTPServer,
			auth,
			config.From,
			config.Recipients,
			message,
		)
		if err != nil {
			return fmt.Errorf("failed to send email: %w", err)
		}
	} else {
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}

		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}

		if err = client.Mail(config.From); err != nil {
			return fmt.Errorf("failed to set sender: %w", err)
		}

		for _, recipient := range config.Recipients {
			if err = client.Rcpt(recipient); err != nil {
				return fmt.Errorf("failed to add recipient %s: %w", recipient, err)
			}
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("failed to get data writer: %w", err)
		}

		_, err = w.Write(message)
		if err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}

		err = w.Close()
		if err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}

		client.Quit()
	}

	return nil
}

// sendWebhookNotification sends a generic webhook notification
func (r *NodeScanReconciler) sendWebhookNotification(ctx context.Context, nodeScan *clamavv1alpha1.NodeScan, scanPolicy *clamavv1alpha1.ScanPolicy) error {
	config := scanPolicy.Spec.Notifications.Webhook

	// Skip if onlyOnInfection and no infections
	if config.OnlyOnInfection && nodeScan.Status.FilesInfected == 0 {
		return nil
	}

	// Build payload
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
	}

	if nodeScan.Status.FilesInfected > 0 {
		var infectedFiles []map[string]interface{}
		for _, f := range nodeScan.Status.InfectedFiles {
			infectedFiles = append(infectedFiles, map[string]interface{}{
				"path":    f.Path,
				"viruses": f.Viruses,
				"size":    f.Size,
			})
		}
		payload["infectedFiles"] = infectedFiles
		payload["severity"] = "critical"
	} else {
		payload["severity"] = "info"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", config.URL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ClamAV-Operator/1.0")

	// Add headers from config
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Add headers from secret
	if config.SecretRef != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      config.SecretRef.Name,
			Namespace: scanPolicy.Namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get webhook secret: %w", err)
		}

		for key, value := range secret.Data {
			req.Header.Set(key, string(value))
		}
	}

	// Send request
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
