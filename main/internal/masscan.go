package internal

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// MasscanResult chứa kết quả quét từ masscan
type MasscanResult struct {
	IP    string `json:"ip"`
	Ports []struct {
		Port     int    `json:"port"`
		Protocol string `json:"proto"`
		Status   string `json:"status"`
		TTL      int    `json:"ttl"`
	} `json:"ports"`
}

// RunMasscan chạy masscan với danh sách IP và dải cổng
// Trả về danh sách kết quả đã parse từ JSON output
func RunMasscan(targets []string, ports string, rate int) ([]MasscanResult, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("danh sách IP rỗng")
	}

	// Kiểm tra masscan có tồn tại không
	masscanPath, err := exec.LookPath("masscan")
	if err != nil {
		return nil, fmt.Errorf("masscan chưa được cài đặt. Vui lòng cài đặt masscan trước")
	}

	// Tạo chuỗi target
	targetStr := strings.Join(targets, ",")

	// Ví dụ lệnh quét alive check:
	// masscan 1.2.3.4 --rate 1000 --open -oJ - -p80,443,8080 --ping
	args := []string{
		targetStr,
		"--rate", fmt.Sprintf("%d", rate),
		"--open",
		"-oJ", "-", // Output JSON ra stdout
	}

	if ports == "" {
		// Ở chế độ "alive check" mặc định, quét các cổng phổ biến để kiểm tra cấu hình
		args = append(args, "-p21,22,80,443,445,6379,7001,8080,8443,9200")
	} else {
		args = append(args, "-p", ports)
	}

	cmd := exec.Command(masscanPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("masscan lỗi: %w\nOutput: %s", err, string(output))
	}

	// Parse JSON output
	results, err := parseMasscanJSON(output)
	if err != nil {
		return nil, fmt.Errorf("lỗi parse kết quả masscan: %w", err)
	}

	return results, nil
}

// parseMasscanJSON parse output JSON của masscan
// Masscan JSON output có dạng: [{...},\n{...},\n]
func parseMasscanJSON(data []byte) ([]MasscanResult, error) {
	raw := string(data)
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var results []MasscanResult

	// Masscan có thể in text linh tinh đan xen vào output.
	// Thay vì cố substring từ [ đến ] vốn rất dễ lỗi nát chuỗi,
	// chúng ta cắt từng dòng và parse từng JSON object riêng lẻ
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Bỏ qua các dòng log rác, json mảng
		if line == "" || line == "[" || line == "]" {
			continue
		}

		// Xóa dấu phẩy ở cuối dòng nếu có để được JSON object chuẩn
		line = strings.TrimSuffix(line, ",")

		// Chỉ parse các dòng bắt đầu bằng { và kết thúc bằng }
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var res MasscanResult
			if err := json.Unmarshal([]byte(line), &res); err == nil {
				// Chỉ thêm nếu có data hợp lệ
				if res.IP != "" {
					results = append(results, res)
				}
			}
		}
	}

	return results, nil
}
