package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	key := flag.String("key", os.Getenv("PRIVACY_AUDIT_KEY"), "HMAC-SHA256 签名密钥 (或 PRIVACY_AUDIT_KEY 环境变量)")
	keyFile := flag.String("key-file", "", "从文件读取 HMAC 密钥")
	logFile := flag.String("log-file", os.Getenv("PRIVACY_BUDGET_AUDIT_LOG"), "审计日志文件路径")
	flag.Parse()

	if *keyFile != "" {
		data, err := os.ReadFile(*keyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 无法读取密钥文件 %s: %v\n", *keyFile, err)
			os.Exit(2)
		}
		*key = strings.TrimSpace(string(data))
	}

	if *key == "" {
		fmt.Fprintf(os.Stderr, "错误: 必须提供 HMAC 密钥 (--key, --key-file 或 PRIVACY_AUDIT_KEY)\n")
		os.Exit(2)
	}

	if *logFile == "" {
		*logFile = "/tmp/budget_audit.log"
	}

	f, err := os.Open(*logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 无法打开审计日志文件 %s: %v\n", *logFile, err)
		os.Exit(2)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	validCount := 0
	invalidCount := 0

	fmt.Printf("开始校验审计日志: %s\n", *logFile)
	fmt.Printf("========================================================\n")

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			fmt.Printf("[行 %d] 格式错误: %s\n", lineNum, line)
			invalidCount++
			continue
		}

		sig := parts[len(parts)-1]
		payload := strings.Join(parts[:len(parts)-1], "|")

		mac := hmac.New(sha256.New, []byte(*key))
		mac.Write([]byte(payload))
		expectedSig := hex.EncodeToString(mac.Sum(nil))

		if hmac.Equal([]byte(sig), []byte(expectedSig)) || sig == expectedSig {
			validCount++
		} else {
			fmt.Printf("[行 %d] ❌ 签名不匹配! 载荷: %s, 实际签名: %s, 期望: %s\n", lineNum, payload, sig, expectedSig)
			invalidCount++
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "读取文件错误: %v\n", err)
		os.Exit(2)
	}

	fmt.Printf("========================================================\n")
	fmt.Printf("校验汇总: 共读取 %d 行, 有效签名: %d, 无效/篡改: %d\n", lineNum, validCount, invalidCount)

	if invalidCount > 0 {
		fmt.Println("❌ 存在签名不匹配或格式错误的记录！")
		os.Exit(1)
	}

	fmt.Println("✅ 全部记录签名校验通过，数据完整无篡改。")
	os.Exit(0)
}
