package mail

import (
	"log"
	"net/smtp"
)

type MailServer struct {
	Address  string
	User     string
	Pass     string
}

func New(Address string, User string, Pass string) (*MailServer, error) {
	ms := MailServer {
		Address: Address,
		User: User,
		Pass: Pass,
	}
	
	return &ms, nil
}

func (ms *MailServer) SendMail (from string, to []string, body string) (error) {
	auth := smtp.PlainAuth("", ms.User, ms.Pass, ms.Address)

	msg := []byte("To: " + to[0] + "\r\n" +
		"Subject: AIDE Service Notification\r\n" +
		"\r\n" + body + "\r\n" +
		"-- AIDE TechSupport\r\n")

	err := smtp.SendMail(ms.Address, auth, from, to, msg)
	if err != nil {
		log.Fatal(err)
		return err
	}
	log.Println("smtp msg sent OK!")
	return nil
}
