package main

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

type MailRequest struct {
	XMLName xml.Name ` + "`xml:"mail"`" + `
	To      string   ` + "`xml:"to"`" + `
	Subject string   ` + "`xml:"subject"`" + `
	Body    string   ` + "`xml:"body"`" + `
}

type MailTask struct {
	ID        int
	To        string
	Subject   string
	Body      string
	FailCount int
}

var (
	db          *sql.DB
	workerCount = 5
	batchSize   = 10
	maxRetries  = 3
)

func main() {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")

	if smtpHost == "" || smtpPort == "" || smtpUser == "" || smtpPass == "" {
		log.Fatal("缺少 SMTP 配置环境变量")
	}

	var err error
	db, err = sql.Open("duckdb", "file:queue.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	initDB()

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(i+1, smtpHost, smtpPort, smtpUser, smtpPass, &wg)
	}

	http.HandleFunc("/send", handleSend)

	log.Println("HTTP 服务已启动，监听 :8080")
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	wg.Wait()
}

func initDB() {
	_, err := db.Exec(` + "`" + `
		CREATE TABLE IF NOT EXISTS queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			to_addr TEXT,
			subject TEXT,
			body TEXT,
			status TEXT DEFAULT 'pending',
			fail_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	` + "`" + `)
	if err != nil {
		log.Fatal(err)
	}
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "读取请求体失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req MailRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		http.Error(w, "XML 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.To == "" || req.Subject == "" || req.Body == "" {
		http.Error(w, "缺少必要字段: to, subject, body", http.StatusBadRequest)
		return
	}

	_, err = db.Exec("INSERT INTO queue (to_addr, subject, body, status, fail_count) VALUES (?, ?, ?, 'pending', 0)",
		req.To, req.Subject, req.Body)
	if err != nil {
		http.Error(w, "数据库错误:"+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("任务已加入队列"))
}

func worker(id int, smtpHost, smtpPort, smtpUser, smtpPass string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		tasks, err := fetchBatchTasks(batchSize)
		if err != nil {
			log.Printf("Worker %d 获取任务错误: %v", id, err)
			time.Sleep(time.Second)
			continue
		}
		if len(tasks) == 0 {
			time.Sleep(time.Second)
			continue
		}
		for _, task := range tasks {
			err := sendMail(task, smtpHost, smtpPort, smtpUser, smtpPass)
			if err != nil {
				log.Printf("Worker %d 发送失败，任务ID=%d: %v", id, task.ID, err)
				incrementFailCount(task.ID)
			} else {
				markTaskSent(task.ID)
				log.Printf("Worker %d 成功发送邮件给 %s, 任务ID=%d", id, task.To, task.ID)
			}
		}
	}
}

func fetchBatchTasks(limit int) ([]MailTask, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT id, to_addr, subject, body, fail_count FROM queue WHERE status='pending' ORDER BY id LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	var tasks []MailTask
	for rows.Next() {
		var t MailTask
		if err := rows.Scan(&t.ID, &t.To, &t.Subject, &t.Body, &t.FailCount); err != nil {
			return nil, err
		}
		ids = append(ids, t.ID)
		tasks = append(tasks, t)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	inClause := ""
	for i, id := range ids {
		if i > 0 {
			inClause += ","
		}
		inClause += strconv.Itoa(id)
	}

	_, err = tx.Exec(fmt.Sprintf("UPDATE queue SET status='processing' WHERE id IN (%s)", inClause))
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

func sendMail(task MailTask, host, port, user, pass string) error {
	portInt, _ := strconv.Atoi(port)
	auth := smtp.PlainAuth("", user, pass, host)

	msg := []byte(fmt.Sprintf("To: %s
Subject: %s

%s
", task.To, task.Subject, task.Body))

	return smtp.SendMail(fmt.Sprintf("%s:%d", host, portInt), auth, user, []string{task.To}, msg)
}

func incrementFailCount(id int) {
	_, err := db.Exec("UPDATE queue SET status='pending', fail_count = fail_count + 1 WHERE id=? AND fail_count < ?", id, maxRetries)
	if err != nil {
		log.Printf("更新失败次数失败，任务ID=%d: %v", id, err)
	}
}

func markTaskSent(id int) {
	_, err := db.Exec("UPDATE queue SET status='sent' WHERE id=?", id)
	if err != nil {
		log.Printf("标记任务已发送失败，任务ID=%d: %v", id, err)
	}
}
