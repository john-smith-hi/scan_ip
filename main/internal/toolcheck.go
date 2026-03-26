package internal

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
)

// ToolInfo chứa thông tin về một tool bên ngoài
type ToolInfo struct {
	Name        string
	Command     string // Lệnh kiểm tra tồn tại
	VersionFlag string // Flag lấy version
	InstallHint string // Hướng dẫn cài đặt
	Required    bool   // Bắt buộc phải có
}

// GetRequiredTools trả về danh sách tool cần kiểm tra
func GetRequiredTools() []ToolInfo {
	currOS := runtime.GOOS

	masscanInstall := "sudo apt install masscan"
	mariadbInstall := "sudo apt install mariadb-server && sudo systemctl start mariadb"

	switch currOS {
	case "windows":
		masscanInstall = "choco install masscan HOẶC tải từ https://github.com/robertdavidgraham/masscan/releases"
		mariadbInstall = "choco install mariadb HOẶC tải từ https://mariadb.org/download/"
	case "darwin":
		masscanInstall = "brew install masscan"
		mariadbInstall = "brew install mariadb && brew services start mariadb"
	}

	return []ToolInfo{
		{
			Name:        "masscan",
			Command:     "masscan",
			VersionFlag: "--version",
			InstallHint: masscanInstall,
			Required:    true,
		},
		{
			Name:        "MariaDB (mysql client)",
			Command:     "mysql",
			VersionFlag: "--version",
			InstallHint: mariadbInstall,
			Required:    true,
		},
		{
			Name:        "mysqldump",
			Command:     "mysqldump",
			VersionFlag: "--version",
			InstallHint: mariadbInstall,
			Required:    true,
		},
		{
			Name:        "nmap",
			Command:     "nmap",
			VersionFlag: "--version",
			InstallHint: "sudo apt install nmap",
			Required:    true,
		},
	}
}

// CheckTool kiểm tra xem tool có tồn tại trong PATH không
func CheckTool(tool ToolInfo) (installed bool, version string) {
	path, err := exec.LookPath(tool.Command)
	if err != nil {
		return false, ""
	}

	// Lấy version
	cmd := exec.Command(path, tool.VersionFlag)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, "(không lấy được version)"
	}

	ver := strings.TrimSpace(string(output))
	// Lấy dòng đầu tiên
	if idx := strings.Index(ver, "\n"); idx > 0 {
		ver = ver[:idx]
	}
	return true, strings.TrimSpace(ver)
}

// CheckAllTools kiểm tra tất cả tool và in kết quả
// Trả về false nếu có tool bắt buộc bị thiếu
func CheckAllTools() bool {
	tools := GetRequiredTools()
	allOK := true
	var missingTools []ToolInfo

	fmt.Println("=== Kiểm tra công cụ cần thiết ===")
	fmt.Println()

	for _, tool := range tools {
		installed, version := CheckTool(tool)

		if installed {
			fmt.Printf("  ✓ %-25s : %s\n", tool.Name, version)
		} else {
			status := "THIẾU"
			if tool.Required {
				status = "THIẾU (BẮT BUỘC)"
				allOK = false
				missingTools = append(missingTools, tool)
			}
			fmt.Printf("  ✗ %-25s : %s\n", tool.Name, status)
			fmt.Printf("    → Cài đặt: %s\n", tool.InstallHint)
		}
	}

	fmt.Println()

	if !allOK && runtime.GOOS == "linux" {
		fmt.Printf("Phát hiện %d công cụ còn thiếu. Bạn có muốn hệ thống tự động cài đặt không? (y/n): ", len(missingTools))
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) == "y" {
			AutoInstallTools(missingTools)
			// Kiểm tra lại sau khi cài đặt
			return CheckAllTools()
		}
	}

	if !allOK {
		log.Println("⚠ Một số công cụ bắt buộc chưa được cài đặt. Vui lòng cài đặt thủ công trước khi chạy.")
	} else {
		fmt.Println("✓ Tất cả công cụ đã sẵn sàng!")
	}

	return allOK
}

// AutoInstallTools cố gắng cài đặt các công cụ bị thiếu trên Linux
func AutoInstallTools(tools []ToolInfo) {
	fmt.Println("--- Đang bắt đầu quá trình cài đặt tự động ---")
	for _, tool := range tools {
		fmt.Printf("Đang cài đặt %s...\n", tool.Name)
		// Trên Linux (Debian/Kali), chúng ta dùng apt
		installCmd := ""
		switch tool.Command {
		case "masscan":
			installCmd = "sudo apt update && sudo apt install -y masscan"
		case "mysql", "mysqldump":
			installCmd = "sudo apt update && sudo apt install -y mariadb-client mariadb-server"
		case "nmap":
			installCmd = "sudo apt update && sudo apt install -y nmap"
		}

		if installCmd != "" {
			cmd := exec.Command("bash", "-c", installCmd)
			output, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("  ✗ Lỗi cài đặt %s: %v\n", tool.Name, err)
				fmt.Printf("    Chi tiết: %s\n", string(output))
			} else {
				fmt.Printf("  ✓ Cài đặt %s thành công!\n", tool.Name)
			}
		}
	}
	fmt.Println("--- Hoàn tất quá trình cài đặt ---")
}

// CheckMariaDBService kiểm tra MariaDB có đang chạy không bằng cách thử kết nối
func CheckMariaDBService(cfg *DBConfig) error {
	db, err := InitDB(cfg)
	if err != nil {
		return fmt.Errorf("MariaDB không phản hồi tại %s:%s — %w", cfg.Host, cfg.Port, err)
	}
	db.Close()
	return nil
}
