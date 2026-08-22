# Hướng dẫn build Video HTML Downloader trên macOS

Có thể copy toàn bộ thư mục dự án `motchill` từ Windows sang MacBook bằng USB, ổ cứng ngoài, mạng nội bộ hoặc Git.

Ứng dụng được cấu hình dạng Universal, chạy trên cả:

- Mac Intel.
- Mac Apple Silicon M1, M2, M3, M4 và các đời mới hơn.
- macOS 12 trở lên, bao gồm macOS 13.

## 1. Những thư mục không cần copy từ Windows

Để giảm dung lượng, có thể bỏ qua:

```text
dist/
dist-macos/
wails-app/frontend/node_modules/
wails-app/frontend/dist/
wails-app/build/bin/
```

Nếu đã copy nguyên thư mục thì vẫn không sao. Phần `node_modules` của Windows nên được xóa và cài lại trên macOS theo bước 4.

## 2. Cài công cụ dòng lệnh của Apple

Mở Terminal trên MacBook và chạy:

```bash
xcode-select --install
```

Chọn **Install** trong cửa sổ hiện ra. Sau khi cài xong, kiểm tra:

```bash
xcode-select -p
```

## 3. Cài Go, Node.js, FFmpeg và Wails v2

Nếu máy đã có Homebrew:

```bash
brew install go node ffmpeg
```

Sau đó cài đúng phiên bản Wails của dự án:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
export PATH="$PATH:$(go env GOPATH)/bin"
wails doctor
```

Nếu lệnh `wails` không được nhận diện sau khi mở Terminal mới, thêm dòng sau vào `~/.zshrc`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Sau đó tải lại cấu hình:

```bash
source ~/.zshrc
```

## 4. Chuẩn bị mã nguồn đã copy từ Windows

Ví dụ thư mục dự án nằm ở Desktop:

```bash
cd ~/Desktop/motchill
```

Nếu thư mục `wails-app/frontend/node_modules` được copy từ Windows, xóa đúng thư mục phụ thuộc đó và cài lại cho macOS:

```bash
rm -rf "$PWD/wails-app/frontend/node_modules"
cd wails-app/frontend
npm install
cd ../..
```

Không chạy lệnh xóa trên nếu Terminal chưa đứng đúng trong thư mục dự án `motchill`.

## 5. Build ứng dụng macOS

Từ thư mục gốc `motchill`, chạy:

```bash
bash build-macos.sh
```

Mỗi lần chạy lệnh trên, số phiên bản tự tăng thêm một bậc mà không cần thao tác gì thêm.

Script sẽ tự động:

1. Tăng số phiên bản trong `VERSION`, `wails-app/version.go` và `wails-app/wails.json`.
2. Build frontend React/TypeScript nếu `wails-app/frontend/dist` chưa tồn tại.
3. Chạy kiểm thử Go.
4. Build ứng dụng Wails Universal cho Intel và Apple Silicon.
5. Ký ứng dụng bằng chữ ký ad-hoc.
6. Đóng gói thành file ZIP và file DMG.

Kết quả nằm tại thư mục `dist-macos`, với `1.0.28` là số phiên bản hiện tại:

```text
dist-macos/VideoHtmlDownloader-v1.0.28-macOS-12-universal.dmg
dist-macos/VideoHtmlDownloader-v1.0.28-macOS-12-universal.zip
```

Hai file chứa cùng một ứng dụng. Dùng file DMG để cài đặt, file ZIP để gửi qua mạng.

Chỉ khi cần build lại mà giữ nguyên số phiên bản, ví dụ lúc thử lại một bản build lỗi, mới đặt thêm biến môi trường:

```bash
BUMP_VERSION=0 bash build-macos.sh
```

## 6. Cài và mở ứng dụng

Mở file DMG, sau đó kéo `VideoHtmlDownloader.app` vào thư mục **Applications** ngay trong cửa sổ vừa hiện ra. Nếu dùng file ZIP thì giải nén rồi kéo tương tự.

Ở lần mở đầu tiên, nếu macOS cảnh báo ứng dụng chưa đến từ nhà phát triển đã xác minh:

1. Nhấp chuột phải vào `VideoHtmlDownloader.app`.
2. Chọn **Open/Mở**.
3. Chọn **Open/Mở** lần nữa trong hộp thoại xác nhận.

FFmpeg đã được cài bằng Homebrew ở bước 3. Nếu ứng dụng chưa tự nhận diện, chọn file:

```text
/opt/homebrew/bin/ffmpeg
```

Trên một số máy Mac Intel, đường dẫn có thể là:

```text
/usr/local/bin/ffmpeg
```

## 7. Ý nghĩa các thư mục kết quả

Quá trình build sinh ra ba thư mục. Cả ba đều nằm trong `.gitignore` nên không được đưa lên Git.

| Thư mục | Vai trò |
| --- | --- |
| `wails-app/frontend/dist` | Giao diện React sau khi Vite biên dịch. Đây là kết quả trung gian, được nhúng thẳng vào file thực thi Go. |
| `wails-app/build/bin` | Nơi Wails đặt `VideoHtmlDownloader.app` vừa biên dịch. Cũng là kết quả trung gian. |
| `dist-macos` | Thư mục phát hành của macOS, chứa file DMG và file ZIP để cài đặt hoặc gửi cho người khác. |

Ngoài ra còn thư mục `dist` ở gốc dự án. Đó là thư mục phát hành của Windows do `build.cmd` tạo ra và chỉ chứa file `.exe`, không liên quan đến bản macOS.

Xóa cả ba thư mục trên đều an toàn, lần build kế tiếp sẽ tạo lại đầy đủ.

## 8. Build lại sau khi sửa code

Chỉ cần mở Terminal tại thư mục dự án và chạy lại:

```bash
bash build-macos.sh
```

Mỗi lần build, số patch sẽ tự tăng (`1.0.28` → `1.0.29`) và tạo file DMG cùng file ZIP có số phiên bản mới trong `dist-macos`.

## Xử lý lỗi thường gặp

### Không tìm thấy lệnh `wails`

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

### `wails doctor` báo thiếu Xcode Command Line Tools

```bash
xcode-select --install
```

### Lỗi package npm được copy từ Windows

Đảm bảo Terminal đang đứng tại thư mục `motchill`, rồi chạy:

```bash
rm -rf "$PWD/wails-app/frontend/node_modules"
cd wails-app/frontend
npm install
cd ../..
bash build-macos.sh
```

### Ứng dụng không tìm thấy FFmpeg

```bash
brew install ffmpeg
which ffmpeg
```

Sau đó dùng nút **Chọn file** trong ứng dụng và chọn đường dẫn do `which ffmpeg` trả về.
