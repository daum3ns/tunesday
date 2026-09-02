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

// SendMagicKeyEmail welcomes a new member with their permanent login link.
func (s *Service) SendMagicKeyEmail(to, teamName, joinURL string) error {
	data := struct {
		TeamName string
		URL      string
	}{
		TeamName: teamName,
		URL:      joinURL,
	}
	subject := "Your permanent Tunesday key"
	body, err := render(magicKeyTmpl, data)
	if err != nil {
		return err
	}
	return s.send(to, subject, body)
}

// TeamLink pairs a team name with its join URL for login emails.
type TeamLink struct {
	TeamName string
	URL      string
}

// SendLoginLinkEmail delivers magic links for self-service login.
func (s *Service) SendLoginLinkEmail(to string, links []TeamLink) error {
	data := struct{ Links []TeamLink }{Links: links}
	subject := "Your tunesday.fm login links"
	body, err := render(loginLinkTmpl, data)
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

var magicKeyTmpl = template.Must(template.New("magickey").Parse(`Welcome to "{{.TeamName}}"!

Here is your permanent Tunesday key. Click it (or bookmark it) whenever you
want to log in — no password needed:

{{.URL}}

This link works on any device and never expires until a team admin removes
you from the team. Lost it? Visit the login page and choose
"Email me my magic link".
`))

var loginLinkTmpl = template.Must(template.New("loginlink").Parse(`Here are your tunesday.fm login links:

{{range .Links}}- {{.TeamName}}: {{.URL}}
{{end}}
Each link logs you in directly. No passwords needed.
`))
