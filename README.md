# Scan IP Toolkit

Tool quét IP tự động sử dụng Go + Masscan + MariaDB.

## Yêu cầu

| Tool     | Version  | Mục đích                        |
|----------|----------|---------------------------------|
| Go       | 1.21+    | Ngôn ngữ chính                  |
| Masscan  | 1.3+     | Quét cổng tốc độ cao            |
| MariaDB  | 10.11+   | Lưu trữ kết quả                 |
| mysqldump| 10.11+   | Sao lưu và nén database         |

Kiểm tra nhanh: `go run ./main check`

## Cài đặt

```bash
# 1. Clone project
git clone <repo-url> scan_ip && cd scan_ip

# 2. Cài dependency Go
go mod tidy

# 3. Cấu hình database
# Sửa file config/database.txt cho phù hợp

# 4. Kiểm tra tool
go run ./main check
```

## Cách dùng

### Kiểm tra công cụ
```bash
go run ./main check
```
→ Tự động kiểm tra masscan, MariaDB có sẵn sàng không.

### Khởi tạo Database
```bash
go run ./main init
```
→ Tạo database `scan_ip` và 3 bảng: `isp_ranges`, `hosts`, `services`.

### Nhập dải IP
```bash
# Mặc định đọc data/vietnam.txt
go run ./main import

# Hoặc chỉ định file khác
go run ./main import -file data/custom.txt
```
File CIDR có định dạng mỗi dòng 1 dải, ví dụ:
```
1.0.128.0/17
1.52.0.0/15
14.160.0.0/11
```

### Quét IP (Scan)

Lệnh `scan` sẽ tự động chọn công cụ quét dựa trên tham số truyền vào (**Hybrid Mode**):

1.  **Chế độ Alive Check (Mặc định - Masscan):**
    *   Sử dụng khi **KHÔNG** truyền `-ports`.
    *   Tự động quét các cổng phổ biến (`21,22,80,443,445,6379,7001,8080,8443,9200`) với `--rate 3000`.
    *   Mục đích: Tìm IP đang sống cực nhanh.
    ```bash
    go run ./main scan -limit 1000
    ```

2.  **Chế độ Port Scan (Nmap):**
    *   Sử dụng khi **CÓ** truyền tham số `-ports`.
    *   Sử dụng **Nmap** (`-sS -Pn -n -T3`) để quét chính xác các cổng yêu cầu.
    *   Mục đích: Quét sâu, tránh ồn ào (stealthy).
    ```bash
    go run ./main scan -limit 100 -ports 80,443,8080
    ```

**Tham số:**
- `-limit`: Số lượng IP tối đa quét mỗi lần (mặc định: 100,000).
- `-ports`: Dải cổng (mặc định: TRỐNG -> dùng Masscan Alive Check).
- `-rate`: Tốc độ masscan (mặc định: 3000).
- `-workers`: Số worker pool chạy song song (mặc định: 10).

> ⚠️ Masscan yêu cầu quyền **root/sudo** (Linux) hoặc **Administrator** (Windows).

### Sao lưu Database (Backup)

Lệnh `backup` giúp bạn sao lưu toàn bộ dữ liệu hiện có phòng trường hợp sự cố hoặc trước khi reset:

```bash
go run ./main backup
```
→ Kết quả sẽ được nén thành file `.sql.gz` lưu trong thư mục `backup_database/`.

### Khôi phục Database (Restore)

Để nạp lại dữ liệu từ file nén:

```bash
go run ./main restore -file backup_database/backup_file_name.sql.gz
```
> ⚠️ Hành động này sẽ xóa sạch dữ liệu hiện tại trong database và thay thế bằng dữ liệu từ file backup. Hệ thống sẽ yêu cầu xác nhận trước khi làm.

### Reset kết quả (Reset-Scan)

Lệnh `reset-scan` sẽ đưa database về trạng thái chưa quét. Hệ thống sẽ liệt kê các thay đổi và yêu cầu **xác nhận 2 lần** trước khi thực hiện.

```bash
go run ./main reset-scan
```

## Cấu trúc Database

- **isp_ranges** — Danh sách dải IP CIDR thô của nhà mạng.
- **hosts** —ừng IP đơn, trạng thái `is_alive`, và ngày quét cuối.
- **services** — Chi tiết cổng mở, giao thức và trạng thái.

## Build

```bash
# Build ra file thực thi (Linux/macOS)
go build -o scan_ip ./main/main.go

# Chạy trực tiếp
sudo ./scan_ip reset-scan
sudo ./scan_ip scan -limit 5000
```
