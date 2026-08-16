# Video HTML Downloader — Wails v2 + React/TypeScript

Ứng dụng Windows 10/11 và macOS tìm và tải video từ URL trang hoặc mã HTML. Backend Go nhận diện HLS (`.m3u8`), DASH (`.mpd`) và video trực tiếp; frontend React/TypeScript cung cấp danh sách tập và hàng đợi tải tuần tự.

## Chạy ứng dụng

Mở:

```text
dist\VideoHtmlDownloader.exe
```

Quy trình sử dụng:

1. Dán một hoặc nhiều URL (mỗi URL một dòng) rồi chọn **Phân tích & thêm**. Từng link được xử lý tuần tự và thêm vào danh sách hiện có. Có thể dùng **Mở file HTML** hoặc **Dán HTML** nếu trang chặn tải tự động.
2. Mỗi phim được hiển thị thành một nhóm riêng cùng toàn bộ tập và vị trí lưu. Có thể chọn từng tập, từng phim hoặc dùng checkbox **Chọn tất cả**.
3. Chọn các phim cần thiết rồi bấm **Chọn** ở mục thư mục lưu. Thư mục mới chỉ áp dụng cho những phim đang chọn và được ghi nhớ cho lần mở tiếp theo.
4. Cài/chọn FFmpeg ở lần đầu, sau đó bấm **Tải**. Tất cả phim và tập được tải lần lượt; server khác sẽ được thử nếu server ưu tiên lỗi.

Tùy chọn **Bỏ qua file đã có** cho phép tiếp tục một bộ đang tải dở mà không tải lại các tập hoàn chỉnh.
Ứng dụng đọc `episodeVariants` của từng trang tập, kiểm tra canonical, URL stream và fingerprint đầu ra; nếu hai tập trả về cùng video, bản trùng bị từ chối thay vì được lưu nhầm.

## Dữ liệu cục bộ

- Thiết lập và thư mục lưu gần nhất: `%APPDATA%\MotchillDownloader\settings.json`
- FFmpeg do ứng dụng tự tải: `%LOCALAPPDATA%\MotchillDownloader\ffmpeg.exe`
- FFmpeg không được nhúng vào EXE; bản Essentials được tải từ `https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip` khi người dùng yêu cầu.

## Biên dịch

Yêu cầu Go, Node.js/npm và Wails CLI v2.13:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
.\build.cmd
```

Mã Wails nằm trong `wails-app`. `build.cmd` chạy test Go, build React/TypeScript và chép EXE hoàn chỉnh vào `dist`.

### macOS

Bản macOS là Universal, chạy trên cả máy Intel và Apple Silicon. Mức tối thiểu được đặt là macOS 12.0, do đó đáp ứng macOS 13 trở lên. Cần build trên macOS (Wails không đóng gói ứng dụng macOS từ Windows):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
bash build-macos.sh
```

File kết quả nằm tại `dist-macos/VideoHtmlDownloader-macOS-12-universal.zip`. Workflow `.github/workflows/build-macos.yml` cũng có thể build thủ công bằng GitHub Actions trên runner macOS. Bản đóng gói dùng chữ ký ad-hoc; khi phát hành công khai nên ký Developer ID và notarize. FFmpeg trên macOS có thể được cài bằng Homebrew ngay trong giao diện hoặc chọn binary có sẵn.

## Giới hạn

- Một số trang yêu cầu cookie đăng nhập hoặc tạo URL bằng JavaScript. Hãy dùng HTML đã lưu sau khi video xuất hiện nếu phân tích URL thất bại.
- Link video có thể hết hạn trong lúc tải cả bộ.
- Ứng dụng không phá DRM. Chỉ tải nội dung bạn sở hữu hoặc được phép lưu.
