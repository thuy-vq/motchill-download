# Wails application

Backend Go:

- `extractor.go`: đọc trang, tách server hiện tại và danh sách tập.
- `downloader.go`: hàng đợi FFmpeg tuần tự, hủy, thử server dự phòng và bỏ qua file đã có.
- `settings.go`: lưu thư mục đầu ra gần nhất và đường dẫn FFmpeg.
- `app.go`: API được bind sang frontend và hộp thoại Windows.

Frontend React/TypeScript nằm trong `frontend/src`.

Kiểm thử bằng file HTML mẫu:

```powershell
$env:MOTCHILL_SAMPLE_HTML='C:\path\to\sample.html'
go test -v ./...
```

Build trực tiếp:

```powershell
wails build -clean -platform windows/amd64 -webview2 embed
```
