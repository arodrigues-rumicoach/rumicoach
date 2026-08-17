package sms

type Provider interface {
	SendSMS(toPhone, content string) error
}
