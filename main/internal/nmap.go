package internal

import (
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"
)

// NmapRun chứa kết quả root của xml nmap
type NmapRun struct {
	Hosts []NmapHost `xml:"host"`
}

type NmapHost struct {
	Addresses []NmapAddress `xml:"address"`
	Ports     NmapPorts     `xml:"ports"`
}

type NmapAddress struct {
	Addr     string `xml:"addr,attr"`
	AddrType string `xml:"addrtype,attr"`
}

type NmapPorts struct {
	Ports []NmapPort `xml:"port"`
}

type NmapPort struct {
	Protocol string    `xml:"protocol,attr"`
	PortID   int       `xml:"portid,attr"`
	State    NmapState `xml:"state"`
}

type NmapState struct {
	State string `xml:"state,attr"`
}

// RunNmap chạy nmap khi quét các port cụ thể
func RunNmap(targets []string, ports string) ([]MasscanResult, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("danh sách IP rỗng")
	}

	nmapPath, err := exec.LookPath("nmap")
	if err != nil {
		return nil, fmt.Errorf("nmap chưa được cài đặt. Vui lòng cài đặt nmap trước")
	}

	targetStr := strings.Join(targets, ",")

	// Lệnh nmap: -sS (SYN), -Pn (No ping), -n (No DNS), -T3 (Normal timing), --min-rate 300
	args := []string{
		"-sS", "-Pn", "-n", "-T3", "--min-rate", "300",
		"-p", ports,
		"-oX", "-", // Output XML ra stdout
		targetStr,
	}

	cmd := exec.Command(nmapPath, args...)
	output, err := cmd.CombinedOutput()
	// nmap có thể exit 1 nếu không có host nào up, nhưng vẫn có output XML hợp lệ
	if err != nil && len(output) == 0 {
		return nil, fmt.Errorf("nmap lỗi không có kết quả: %w", err)
	}

	results, parseErr := parseNmapXML(output)
	if parseErr != nil {
		return nil, fmt.Errorf("lỗi parse kết quả nmap: %w", parseErr)
	}

	return results, nil
}

// parseNmapXML đọc XML và gom thành struct giống MasscanResult để tương thích
func parseNmapXML(data []byte) ([]MasscanResult, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var run NmapRun
	if err := xml.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("không thể unmarshal XML: %w", err)
	}

	var results []MasscanResult

	for _, host := range run.Hosts {
		var ip string
		for _, addr := range host.Addresses {
			if addr.AddrType == "ipv4" || addr.AddrType == "ipv6" {
				ip = addr.Addr
				break
			}
		}

		if ip == "" {
			continue
		}

		var r MasscanResult
		r.IP = ip

		for _, port := range host.Ports.Ports {
			if port.State.State == "open" {
				r.Ports = append(r.Ports, struct {
					Port     int    `json:"port"`
					Protocol string `json:"proto"`
					Status   string `json:"status"`
					TTL      int    `json:"ttl"`
				}{
					Port:     port.PortID,
					Protocol: port.Protocol,
					Status:   port.State.State,
					TTL:      0, // Nmap không cung cấp TTL theo định dạng này
				})
			}
		}

		// Chỉ đẩy vào list results nếu có ít nhất 1 port MỞ (hành vi giống masscan JSON)
		if len(r.Ports) > 0 {
			results = append(results, r)
		}
	}

	return results, nil
}
