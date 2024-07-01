package handler

import (
	"context"
	"fmt"
	mailv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/mail/v1"
	"os"

	"github.com/mailgun/mailgun-go/v4"
	"github.com/matcornic/hermes/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var from = "Server Alpha <team@mail.server-alpha.tech>"
var serviceName = "Team Ganz Weit Weg"

type MailService struct {
	logger *zap.SugaredLogger
	tracer trace.Tracer
	mg     *mailgun.MailgunImpl
	h      *hermes.Hermes
	mailv1.UnimplementedMailServiceServer
}

// NewMailService creates a new mail service
func NewMailService(logger *zap.SugaredLogger) *MailService {
	apiKey := os.Getenv("MAILGUN_API_KEY")
	mailgunInstance := mailgun.NewMailgun("mail.server-alpha.tech", apiKey)
	mailgunInstance.SetAPIBase(mailgun.APIBaseEU)

	return &MailService{
		logger: logger,
		tracer: otel.GetTracerProvider().Tracer("grpc-mail-service"),
		h: &hermes.Hermes{
			Theme:         new(hermes.Default),
			TextDirection: hermes.TDLeftToRight,
			Product: hermes.Product{
				Name:        "Server Alpha",
				Link:        "https://server-alpha.com/",
				Logo:        "https://wallpapercave.com/wp/wp8802810.jpg",
				Copyright:   "© WWI21SEB / Projektkonzeption und Realisierung",
				TroubleText: "If you’re having trouble with the button '{ACTION}', copy and paste the URL below into your web browser.",
			},
		},
		mg: mailgunInstance,
	}
}

func (ms *MailService) SendConfirmationMail(ctx context.Context, request *mailv1.SendConfirmationMailRequest) (*mailv1.SendConfirmationMailResponse, error) {
	email := hermes.Email{
		Body: hermes.Body{
			Name: request.GetUser().GetUsername(),
			Intros: []string{
				fmt.Sprintf("Welcome %s! Your account has been successfully activated.", request.GetUser().GetUsername()),
				"Please note that the registration is completed by Server Alpha. If you have any questions, feel free to reach out to us at any time via team@mail.server-alpha.tech",
			},
			Outros: []string{
				fmt.Sprintf("We thank you again for choosing %s in combination with Server Alpha!", serviceName),
			},
		},
	}
	subject := fmt.Sprintf("Welcome to Server Alpha, %s", request.GetUser().GetUsername())

	ms.logger.Infof("Sending confirmation mail to %s", request.GetUser().GetEmail())
	if err := ms.sendMail(ctx, email, subject, request.GetUser().GetEmail()); err != nil {
		ms.logger.Infof("Error in ms.SendMail: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to send confirmation mail: %v", err)
	}

	return &mailv1.SendConfirmationMailResponse{}, nil
}

func (ms *MailService) SendTokenMail(ctx context.Context, request *mailv1.SendTokenMailRequest) (*mailv1.SendTokenMailResponse, error) {
	var email hermes.Email
	subject := ""

	switch request.GetType() {
	case mailv1.TokenMailType_TOKEN_MAIL_TYPE_REGISTRATION:
		ms.logger.Infof("Sending registration mail to %s", request.GetUser().GetEmail())
		subject = fmt.Sprintf("Welcome %s! Activate your account now!", request.GetUser().GetUsername())
		email = ms.generateRegistrationMail(ctx, request)
	case mailv1.TokenMailType_TOKEN_MAIL_TYPE_PASSWORD_RESET:
		ms.logger.Infof("Sending password reset mail to %s", request.GetUser().GetEmail())
		subject = fmt.Sprintf("Password reset for %s", request.GetUser().GetUsername())
		email = ms.generatePasswordResetMail(ctx, request)
	default:
		ms.logger.Infof("Invalid token mail type: %v", request.GetType())
		return nil, status.Errorf(codes.InvalidArgument, "Invalid token mail type")
	}

	if err := ms.sendMail(ctx, email, subject, request.GetUser().GetEmail()); err != nil {
		ms.logger.Infof("Error in ms.SendMail: %v", err)
		return nil, status.Errorf(codes.Internal, "Failed to send token mail: %v", err)
	}

	return &mailv1.SendTokenMailResponse{}, nil
}

// SendMail sends an email
func (ms *MailService) sendMail(ctx context.Context, email hermes.Email, subject, to string) error {
	_, composeSpan := ms.tracer.Start(ctx, "composeMail")
	emailBody, err := ms.h.GenerateHTML(email)
	if err != nil {
		composeSpan.End()
		ms.logger.Infof("Error in ms.h.GenerateHTML: %v", err)
		return err
	}

	message := ms.mg.NewMessage(from, subject, "", to)
	message.SetHtml(emailBody)
	composeSpan.End()

	sendCtx, sendSpan := ms.tracer.Start(ctx, "sendMail")
	defer sendSpan.End()
	_, _, err = ms.mg.Send(sendCtx, message)
	if err != nil {
		ms.logger.Infof("Error in ms.mg.Send: %v", err)
		return err
	}

	return nil
}

func (ms *MailService) generateRegistrationMail(ctx context.Context, request *mailv1.SendTokenMailRequest) hermes.Email {
	_, span := ms.tracer.Start(ctx, "generateRegistrationMail")
	defer span.End()

	return hermes.Email{
		Body: hermes.Body{
			Name: request.GetUser().GetUsername(),
			Intros: []string{
				fmt.Sprintf("Welcome to %s! We're very excited to have you on board.", serviceName),
				"Please note that the registration is completed by Server Alpha. If you have any questions, feel free to reach out to us at any time via team@mail.server-alpha.tech.",
			},
			Outros: []string{
				fmt.Sprintf("We thank you again for choosing %s in combination with Server Alpha!", serviceName),
			},
			Actions: []hermes.Action{
				{
					Instructions: fmt.Sprintf("To activate your account, please login to %s and enter the following code:", serviceName),
					InviteCode:   request.GetToken(),
				},
			},
		},
	}
}

func (ms *MailService) generatePasswordResetMail(ctx context.Context, request *mailv1.SendTokenMailRequest) hermes.Email {
	_, span := ms.tracer.Start(ctx, "generatePasswordResetMail")
	defer span.End()

	return hermes.Email{
		Body: hermes.Body{
			Name: request.GetUser().GetUsername(),
			Intros: []string{
				"We have received a request to reset your password.",
				"If you did not request this, please ignore this email.",
			},
			Outros: []string{
				"If you have any questions, feel free to reach out to us at any time via team@mail.server-alpha.tech.",
			},
			Actions: []hermes.Action{
				{
					Instructions: "To reset your password, please copy and paste the following code:",
					InviteCode:   request.GetToken(),
				},
			},
		},
	}
}
