package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	// "golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/MihkelHunter/release-notifier/internal/config"
	"github.com/MihkelHunter/release-notifier/internal/markdown"
	"github.com/MihkelHunter/release-notifier/internal/recipients"
)

// Message is the assembled email ready to send.
type Message struct {
	Subject    string
	Body       string // HTML
	Recipients []recipients.Recipient
}

// Build assembles the email HTML from parsed release notes.
func Build(notes *markdown.ParsedNotes, env string, cfg *config.Config, rcpts []recipients.Recipient) (*Message, error) {
	envCfg, ok := cfg.Environments[env]
	prefix := "[RELEASE]"
	if ok && envCfg.SubjectPrefix != "" {
		prefix = envCfg.SubjectPrefix
	}

	subject := fmt.Sprintf("%s %s — %s", prefix, notes.Version, notes.Date)

	body, err := renderEmailTemplate(notes, env)
	if err != nil {
		return nil, fmt.Errorf("rendering email template: %w", err)
	}

	return &Message{
		Subject:    subject,
		Body:       body,
		Recipients: rcpts,
	}, nil
}

// renderEmailTemplate wraps the markdown HTML in a nice email layout.
func renderEmailTemplate(notes *markdown.ParsedNotes, env string) (string, error) {
	const tmpl = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #f5f5f5;
    margin: 0; padding: 20px;
  }
  .container {
    max-width: 680px; margin: 0 auto;
    background: #fff; border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 1px 4px rgba(0,0,0,0.1);
  }
  .header {
    background: #1a1a2e; color: #fff;
    padding: 24px 32px;
  }
  .header h1 { margin: 0; font-size: 22px; font-weight: 600; }
  .header .meta { margin-top: 6px; font-size: 13px; opacity: 0.7; }
  .body { padding: 28px 32px; color: #333; line-height: 1.6; }
  .body h2 { color: #1a1a2e; border-bottom: 2px solid #eee; padding-bottom: 6px; }
  .body h3 { color: #444; }
  .body ul { padding-left: 20px; }
  .body li { margin-bottom: 6px; }
  .body code {
    background: #f0f0f0; padding: 2px 6px;
    border-radius: 3px; font-size: 13px;
  }
  .body pre {
    background: #f6f8fa; border: 1px solid #e1e4e8;
    border-radius: 6px; padding: 16px; overflow-x: auto;
  }
  .tags { margin-top: 8px; }
  .tag {
    display: inline-block; background: #e8f0fe; color: #1967d2;
    border-radius: 12px; padding: 2px 10px;
    font-size: 12px; margin-right: 4px;
  }
  .footer {
    background: #f9f9f9; border-top: 1px solid #eee;
    padding: 16px 32px; font-size: 12px; color: #999;
  }
</style>
</head>
<body>
<div class="container">
  <div class="header">
    <h1>🚀 Release {{ .Version }}</h1>
    <div class="meta">{{ .Env }} · {{ .Date }}</div>
    {{ if .Tags }}
    <div class="tags" style="margin-top:10px;">
      {{ range .Tags }}<span class="tag">{{ . }}</span>{{ end }}
    </div>
    {{ end }}
  </div>
  <div class="body">
    {{ .ContentHTML }}
  </div>
  <div class="footer">
    This notification was generated automatically. Contact the dev team for questions.
  </div>
</div>
</body>
</html>`

	t, err := template.New("email").Parse(tmpl)
	if err != nil {
		return "", err
	}

	data := struct {
		Version     string
		Date        string
		Env         string
		Tags        []string
		ContentHTML template.HTML
	}{
		Version:     notes.Version,
		Date:        notes.Date,
		Env:         strings.ToUpper(env),
		Tags:        notes.Tags,
		ContentHTML: template.HTML(notes.HTML),
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ─── Microsoft Graph Sender ────────────────────────────────────────────────

// GraphSender sends emails via Microsoft Graph API.
type GraphSender struct {
	cfg    *config.Config
	client *http.Client
}

func NewGraphSender(cfg *config.Config) *GraphSender {
	ccCfg := clientcredentials.Config{
		ClientID:     cfg.Azure.ClientID,
		ClientSecret: cfg.Azure.ClientSecret,
		TokenURL:     fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.Azure.TenantID),
		Scopes:       []string{"https://graph.microsoft.com/.default"},
	}
	httpClient := ccCfg.Client(context.Background())
	return &GraphSender{cfg: cfg, client: httpClient}
}

func (s *GraphSender) Send(msg *Message) error {
	toRecipients := make([]graphRecipient, len(msg.Recipients))
	for i, r := range msg.Recipients {
		toRecipients[i] = graphRecipient{
			EmailAddress: graphEmailAddress{
				Name:    r.Name,
				Address: r.Email,
			},
		}
	}

	payload := graphSendMailRequest{
		Message: graphMessage{
			Subject: msg.Subject,
			Body: graphBody{
				ContentType: "HTML",
				Content:     msg.Body,
			},
			ToRecipients: toRecipients,
		},
		SaveToSentItems: true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", s.cfg.Sender)
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending Graph API request: %w", err)
	}
	defer resp.Body.Close()

	// Graph returns 202 Accepted on success
	if resp.StatusCode != http.StatusAccepted {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("Graph API returned %d: %v", resp.StatusCode, errBody)
	}

	return nil
}

// ─── Graph API types ───────────────────────────────────────────────────────

type graphSendMailRequest struct {
	Message         graphMessage `json:"message"`
	SaveToSentItems bool         `json:"saveToSentItems"`
}

type graphMessage struct {
	Subject      string           `json:"subject"`
	Body         graphBody        `json:"body"`
	ToRecipients []graphRecipient `json:"toRecipients"`
}

type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type graphRecipient struct {
	EmailAddress graphEmailAddress `json:"emailAddress"`
}

type graphEmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}
