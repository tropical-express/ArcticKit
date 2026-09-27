package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	file *os.File
	mu   sync.Mutex
)

func Init(root string) error {
	mu.Lock()
	defer mu.Unlock()

	logDir := filepath.Join(root, "logs")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logPath := filepath.Join(logDir, "arctickit.log")

	f, err := os.OpenFile(
		logPath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return err
	}

	file = f

	Info("========================================")
	Info("ArcticKit started")
	Info("Log file: " + logPath)
	Info("========================================")

	return nil
}

func Close() {
	mu.Lock()
	defer mu.Unlock()

	if file != nil {
		_ = file.Close()
		file = nil
	}
}

func log(level, message string) {
	mu.Lock()
	defer mu.Unlock()

	line := fmt.Sprintf(
		"%s [%s] %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		level,
		message,
	)

	if file != nil {
		_, _ = file.WriteString(line)
	}

	fmt.Print(line)
}

func Info(message string) {
	log("INFO", message)
}

func Warn(message string) {
	log("WARN", message)
}

func Error(message string) {
	log("ERROR", message)
}

func Command(command string, args ...string) {
	message := command

	for _, arg := range args {
		message += " " + arg
	}

	log("CMD", message)
}
