package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"sync"
)

type MailRequest struct {
	XMLName xml.Name ` + "`xml:"mail"`" + `
	To      string   ` + "`xml:"to"`" + `
	Subject string   ` + "`xml:"subject"`" + `
	Body    string   ` + "`xml:"body"`" + `
}

type MailTask struct {
	To      string
	Subject string
	Body    string
}

var (
	queue      = make(chan MailTask, 1000)
	workerNum  = 5
	wg         sync.WaitGroup
	smtpHost   string
	smtpPort   string
	smtpUser   string
	smtpPass   string
)

func main() {
	smtpHost = os.Getenv("SMTP_HOST")
	smtpPort = os.Getenv("SMTP_PORT")
	smtpUser = os.Getenv("SMTP_USER")
	smtpPass = os.Getenv("SMTP_PASS")

	if smtpHost == "" || smtpPort == "" || smtpUser == "" || smtpPass == "" {
		log.Fatal("缺少 SMTP 配置环境变量")
	}

	for i := 0; i < workerNum; i++ {
		wg.Add(1)
		go worker(i)
	}

	http.HandleFunc("/send", handleSend)

	log.Println("服务启动，监听 :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}

	wg.Wait()
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct != "application/xml" && ct != "text/xml" {
		http.Error(w, "Content-Type 必须是 application/xml 或 text/xml", http.StatusUnsupportedMediaType)
		return
	}

	var req MailRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "XML 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.To == "" || req.Subject == "" || req.Body == "" {
		http.Error(w, "缺少必要字段", http.StatusBadRequest)
		return
	}

	task := MailTask{To: req.To, Subject: req.Subject, Body: req.Body}

	select {
	case queue <- task:
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("邮件任务已入队列"))
	default:
		http.Error(w, "队列已满，请稍后再试", http.StatusServiceUnavailable)
	}
}

func worker(id int) {
	defer wg.Done()
	for task := range queue {
		err := sendMail(task)
		if err != nil {
			log.Printf("worker %d 发送邮件失败: %v", id, err)
		} else {
			log.Printf("worker %d 成功发送邮件给 %s", id, task.To)
		}
	}
}

func sendMail(task MailTask) error {
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	msg := []byte(fmt.Sprintf("To: %s
Subject: %s

%s
", task.To, task.Subject, task.Body))
	return smtp.SendMail(fmt.Sprintf("%s:%s", smtpHost, smtpPort), auth, smtpUser, []string{task.To}, msg)
}
