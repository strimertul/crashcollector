package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/sethvargo/go-envconfig"
	mail "github.com/xhit/go-simple-mail/v2"
	"go.uber.org/zap"
)

const template = `A crash report has been submitted!`

type Config struct {
	Bind         string `env:"BIND,default=:3000"`
	MailFrom     string `env:"MAIL_FROM,default=crash@strimertul.stream"`
	MailTo       string `env:"MAIL_TO,default=crash@strimertul.stream"`
	MailPassword string `env:"MAIL_PASSWD"`
	MailSubject  string `env:"MAIL_SUBJECT,default=Crash report submitted"`
	MailHost     string `env:"MAIL_HOST,default=uchu.moe"`
	MailPort     int    `env:"MAIL_PORT,default=587"`
}

func readFile(entry *multipart.FileHeader) ([]byte, error) {
	file, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func sendMail(config Config, form *multipart.Form, id string) error {
	server := mail.NewSMTPClient()
	server.Host = config.MailHost
	server.Port = config.MailPort
	server.Username = config.MailFrom
	server.Password = config.MailPassword
	server.Encryption = mail.EncryptionSTARTTLS
	server.KeepAlive = false
	server.ConnectTimeout = 10 * time.Second
	server.SendTimeout = 10 * time.Second
	server.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	smtpClient, err := server.Connect()
	if err != nil {
		return fmt.Errorf("connection error: %w", err)
	}

	email := mail.NewMSG()
	email.SetFrom(fmt.Sprintf("Crash collector <%s>", config.MailFrom)).
		AddTo(config.MailTo).
		SetSubject(fmt.Sprintf("[%s] %s", id, config.MailSubject))

	for field, headers := range form.File {
		for _, entry := range headers {
			name := entry.Filename
			if name == "" {
				name = field
			}
			data, err := readFile(entry)
			if err != nil {
				logger.Warn("could not open file", zap.String("filename", name), zap.Error(err))
				continue
			}
			ext := mime.TypeByExtension(filepath.Ext(name))
			if ext == "" {
				ext = "text/plain"
			}
			email.Attach(&mail.File{
				FilePath: name,
				Name:     name,
				Data:     data,
				MimeType: ext,
				Inline:   false,
			})
		}
	}

	for key, values := range form.Value {
		content := ""
		for _, line := range values {
			content += line
		}

		email.Attach(&mail.File{
			FilePath: key + ".txt",
			Name:     key + ".txt",
			Data:     []byte(content),
			MimeType: "text/plain",
			Inline:   true,
		})
	}

	email.SetBody(mail.TextPlain, template)
	if email.Error != nil {
		return fmt.Errorf("email error: %w", email.Error)
	}

	if err = email.Send(smtpClient); err != nil {
		return fmt.Errorf("send error: %w", err)
	}

	return nil
}

var logger zap.Logger

func main() {
	ctx := context.Background()
	logger, _ := zap.NewProduction()

	var config Config
	if err := envconfig.Process(ctx, &config); err != nil {
		log.Fatal(err)
	}

	if config.MailPassword == "" {
		log.Fatal("MAIL_PASSWD must be set")
	}

	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20 /* 32 MB */); err != nil {
			http.Error(w, "body parse error: "+err.Error(), http.StatusBadRequest)
			return
		}

		id := strings.ToUpper(hex.EncodeToString(append(binary.AppendVarint([]byte{}, time.Now().Unix()), binary.AppendVarint([]byte{}, rand.Int63())[0:4]...)))

		if err := sendMail(config, r.MultipartForm, id); err != nil {
			logger.Error("sending error failed", zap.Error(err))
			http.Error(w, "failed submitting crash report because of a server error", http.StatusInternalServerError)
			return
		}

		fmt.Fprint(w, id)
	})
	log.Fatal(http.ListenAndServe(config.Bind, nil))
}
