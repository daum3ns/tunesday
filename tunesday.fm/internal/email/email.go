package email

import (
	"bytes"
	"fmt"
	"net/smtp"
	"text/template"

	"tunesday/tunesday.fm/internal/config"
)

// Service sends transactional emails via SMTP.
type Service struct {
	cfg      *config.Config
	SendFunc func(to, subject, body string) error
}

// NewService creates an email service from config.
func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) send(to, subject, body string) error {
	if s.SendFunc != nil {
		return s.SendFunc(to, subject, body)
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)
	msg := []byte(fmt.Sprintf(
		"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain; charset=\"utf-8\"\r\n"+
			"\r\n"+
			"%s",
		to, subject, body,
	))
	return smtp.SendMail(addr, smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost), s.cfg.SMTPFrom, []string{to}, msg)
}

// SendVerificationEmail sends a verification link to a newly registered admin.
func (s *Service) SendVerificationEmail(to, token string) error {
	url := fmt.Sprintf("%s/verify?token=%s", s.cfg.BaseURL, token)
	data := struct {
		URL string
	}{
		URL: url,
	}

	subject := "Verify your tunesday.fm account"
	body, err := render(verificationTmpl, data)
	if err != nil {
		return err
	}

	return s.send(to, subject, body)
}

// SendInvitationEmail sends a magic link invitation to a team member.
func (s *Service) SendInvitationEmail(to, teamName, magicURL string) error {
	data := struct {
		TeamName string
		URL      string
	}{
		TeamName: teamName,
		URL:      magicURL,
	}

	subject := fmt.Sprintf("You've been invited to %s on tunesday.fm", teamName)
	body, err := render(invitationTmpl, data)
	if err != nil {
		return err
	}

	return s.send(to, subject, body)
}

func render(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var verificationTmpl = template.Must(template.New("verification").Parse(`Welcome to tunesday.fm!

Please verify your email address by clicking the link below:

{{.URL}}

If you did not register, you can ignore this email.
`))

var invitationTmpl = template.Must(template.New("invitation").Parse(`You've been invited to join the Tunesday team "{{.TeamName}}".

Click the link below to join:

{{.URL}}

This link is valid until revoked by a team admin.
`))
