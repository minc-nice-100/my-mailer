package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	mail "github.com/xhit/go-simple-mail/v2"
)

type MailRequest struct {
	XMLName xml.Name `xml:"mail"`
	To      string   `xml:"to"`
	Subject string   `xml:"subject"`
	Body    string   `xml:"body"`
}

type MailTask struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type SendResponse struct {
	Message string     `json:"message"`
	Queue   []MailTask `json:"queue"`
}

var (
	queue     = make(chan MailTask, 1000)
	workerNum = 5
	wg        sync.WaitGroup

	smtpHost string
	smtpPort string
	smtpUser string
	smtpPass string
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

	resp := SendResponse{}

	select {
	case queue <- task:
		resp.Message = "邮件任务已入队列"
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		resp.Message = "队列已满，请稍后再试"
	}

	resp.Queue = getQueueSnapshot()

	w.Header().Set("Content-Type", "application/json")
	if resp.Message == "队列已满，请稍后再试" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(resp)
}

func getQueueSnapshot() []MailTask {
	var tasks []MailTask
	for {
		select {
		case t := <-queue:
			tasks = append(tasks, t)
		default:
			for _, t := range tasks {
				queue <- t
			}
			return tasks
		}
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
	server := mail.NewSMTPClient()
	server.Host = smtpHost
	port, err := strconv.Atoi(smtpPort)
	if err != nil {
		return fmt.Errorf("无效的 SMTP_PORT: %v", err)
	}
	server.Port = port
	server.Username = smtpUser
	server.Password = smtpPass
	server.Encryption = mail.EncryptionSSLTLS // SMTPS 465端口必须
	server.ConnectTimeout = 10 * time.Second
	server.SendTimeout = 10 * time.Second

	smtpClient, err := server.Connect()
	if err != nil {
		return fmt.Errorf("连接 SMTP 失败: %v", err)
	}

	email := mail.NewMSG()
	email.SetFrom(smtpUser).
		AddTo(task.To).
		SetSubject(task.Subject).
		SetBody(mail.TextPlain, task.Body)

	if email.Error != nil {
		return fmt.Errorf("邮件构建失败: %v", email.Error)
	}

	return email.Send(smtpClient)
}
