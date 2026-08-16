# Video HTML Downloader — Wails v2 + React/TypeScript

Ứng dụng Windows 10/11 và macOS tìm và tải video từ URL trang hoặc mã HTML. Backend Go nhận diện HLS (`.m3u8`), DASH (`.mpd`) và video trực tiếp; frontend React/TypeScript cung cấp danh sách tập và hàng đợi tải tuần tự.
Link: https://motchillu.app/phim/y-thien-do-long-ky-1703867226

## Chạy ứng dụng

Mở:

```text
dist\VideoHtmlDownloader.exe
```

Quy trình sử dụng:

1. Dán một hoặc nhiều URL (mỗi URL một dòng) rồi chọn **Phân tích & thêm**. Từng link được xử lý tuần tự và thêm vào danh sách hiện có. Có thể dùng **Mở file HTML** hoặc **Dán HTML** nếu trang chặn tải tự động.
2. Mỗi phim được hiển thị thành một nhóm riêng cùng toàn bộ tập và vị trí lưu. Có thể chọn từng tập, từng phim hoặc dùng checkbox **Chọn tất cả**. Bấm vào tên phim hoặc nút mũi tên để thu gọn/mở rộng danh sách tập của nhóm đó; nút **Thu gọn tất cả** gập toàn bộ danh sách. Khi thu gọn, dòng tiêu đề vẫn hiện số tập hoàn tất, bỏ qua và lỗi.
3. Chọn các phim cần thiết rồi bấm **Chọn** ở mục thư mục lưu. Thư mục mới chỉ áp dụng cho những phim đang chọn và được ghi nhớ cho lần mở tiếp theo.
4. Cài/chọn FFmpeg ở lần đầu, sau đó bấm **Tải**. Tất cả phim và tập được tải lần lượt; server khác sẽ được thử nếu server ưu tiên lỗi.

Nhật ký được **lưu tự động**: mỗi dòng hiện trên màn hình đều được ghi ngay xuống file nên không mất log khi ứng dụng đóng đột ngột. Mỗi lần bấm **Tải** sẽ mở một file mới trong:

- Windows: `%APPDATA%\MotchillDownloader\logs`
- macOS: `~/Library/Application Support/MotchillDownloader/logs`

Nút **📂 Thư mục log** mở thư mục này, nút **💾 Lưu bản sao** xuất thêm một bản ra vị trí tự chọn. Ứng dụng chỉ giữ lại 20 file log gần nhất. Nhật ký bao gồm phiên bản ứng dụng, lúc bắt đầu và kết quả của từng tập, thông báo lỗi cùng đường dẫn file đầu ra. Log được lưu dạng UTF-8 để hiển thị đúng tiếng Việt trên Windows và macOS.

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
Mỗi lần chạy `build.cmd`, số patch trong file `VERSION` được tự tăng và kết quả có cả tên phiên bản, ví dụ `dist/VideoHtmlDownloader-v1.0.1.exe`. File `dist/VideoHtmlDownloader.exe` luôn là bản mới nhất.

### macOS

Bản macOS là Universal, chạy trên cả máy Intel và Apple Silicon. Mức tối thiểu được đặt là macOS 12.0, do đó đáp ứng macOS 13 trở lên. Cần build trên macOS (Wails không đóng gói ứng dụng macOS từ Windows):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
bash build-macos.sh
```

File kết quả có dạng `dist-macos/VideoHtmlDownloader-v1.0.1-macOS-12-universal.zip`. Workflow `.github/workflows/build-macos.yml` cũng có thể build thủ công bằng GitHub Actions trên runner macOS. Bản đóng gói dùng chữ ký ad-hoc; khi phát hành công khai nên ký Developer ID và notarize. FFmpeg trên macOS có thể được cài bằng Homebrew ngay trong giao diện hoặc chọn binary có sẵn.

## Giới hạn

- Một số trang yêu cầu cookie đăng nhập hoặc tạo URL bằng JavaScript. Hãy dùng HTML đã lưu sau khi video xuất hiện nếu phân tích URL thất bại.
- Link video có thể hết hạn trong lúc tải cả bộ.
- Ứng dụng không phá DRM. Chỉ tải nội dung bạn sở hữu hoặc được phép lưu.
