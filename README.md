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
3. Chọn thư mục lưu theo ba mức: nút **Chọn** ở khung thiết lập áp cho mọi phim đang chọn, nút 📁 ở dòng phim áp cho riêng phim đó, nút 📁 ở dòng tập chỉ áp cho tập đó. Thư mục của tập luôn được ưu tiên hơn thư mục của phim.
4. Cài/chọn FFmpeg ở lần đầu, sau đó bấm **Tải**. Tất cả phim và tập được tải lần lượt; server khác sẽ được thử nếu server ưu tiên lỗi. Có thể bật **Tắt máy khi tải xong** — máy tắt sau 60 giây kể từ lúc hàng đợi kết thúc và luôn có nút hủy.
5. Thẻ **Kết quả** bên cạnh **Nhật ký** liệt kê theo từng phim: tập nào hoàn tất (kèm nơi lưu), tập nào lỗi (kèm lý do), tập nào chưa tải; có bộ lọc chỉ hiện lỗi và nút chọn đúng các tập lỗi để tải lại.

Nút **❔ Hướng dẫn** trên thanh tiêu đề mở phần trợ giúp trong ứng dụng: các bước sử dụng, danh sách nguồn được hỗ trợ, phiên bản và ngày build.

Danh sách phim được ghi nhớ vào `%APPDATA%\MotchillDownloader\session.json` cùng thư mục lưu và kết quả từng tập. Nếu lần trước còn tập lỗi hoặc chưa tải, lần mở kế tiếp ứng dụng sẽ hỏi có mở lại danh sách đó không.

Nhật ký được **lưu tự động**: mỗi dòng hiện trên màn hình đều được ghi ngay xuống file nên không mất log khi ứng dụng đóng đột ngột. Mỗi lần bấm **Tải** sẽ mở một file mới trong:

- Windows: `%APPDATA%\MotchillDownloader\logs`
- macOS: `~/Library/Application Support/MotchillDownloader/logs`

Nút **📂 Thư mục log** mở thư mục này, nút **💾 Lưu bản sao** xuất thêm một bản ra vị trí tự chọn. Ứng dụng chỉ giữ lại 20 file log gần nhất. Nhật ký bao gồm phiên bản ứng dụng, lúc bắt đầu và kết quả của từng tập, thông báo lỗi cùng đường dẫn file đầu ra. Log được lưu dạng UTF-8 để hiển thị đúng tiếng Việt trên Windows và macOS.

Mỗi tập đang tải có thanh tiến độ riêng kèm phần trăm, thời lượng đã xử lý và tốc độ. Khi không đọc được tổng thời lượng, thanh chuyển sang dạng chạy liên tục thay vì hiện số phần trăm sai.

**Nhiều server cho mỗi tập:** danh sách tập gom link của **tất cả** server mà host cung cấp, xếp server của trang đang mở lên đầu. Một server trả 404 thì tập đó tự chuyển sang server khác; khi mọi server đã lưu đều lỗi, ứng dụng mở lại trang tập để lấy link mới rồi thử tiếp. Ô **Server ưu tiên** liệt kê mọi server tìm được nên có thể ép dùng một server cụ thể.

**Chống treo:** nếu FFmpeg không tiến triển thêm giây nào trong 90 giây (không tính lúc đang tạm dừng), tiến trình bị tắt và tập đó được tải lại, tối đa 3 lần trước khi thử server khác. Tiến trình FFmpeg được gắn vào job object của Windows nên không thể sống sót khi ứng dụng đóng hoặc bị tắt đột ngột.

Tùy chọn **Bỏ qua file đã có** cho phép tiếp tục một bộ đang tải dở mà không tải lại các tập hoàn chỉnh.
Ứng dụng đọc `episodeVariants` của từng trang tập, kiểm tra canonical, URL stream và fingerprint đầu ra; nếu hai tập trả về cùng video, bản trùng bị từ chối thay vì được lưu nhầm.

## Nguồn được hỗ trợ

Ngoài `motchill`, ứng dụng nhận các host cùng mã nguồn như `motchill.credit`, `motphimchill.cc`, `motphimchilll.me`, `phimmoichill.hair`. Chúng chỉ khác nhau ở dạng URL, và cả bốn dạng dưới đây đều được xử lý qua cùng một bộ nhận diện:

| Dạng link | Ví dụ |
| --- | --- |
| Trang phim | `/phim/xieu-long-giang-sinh`, `/phim/luc-luong-tinh-nhue` |
| Trang phim, tập nằm ở prefix khác | `/phim/gantz` → `/xem-phim/gantz/tap-1/vietsub` |
| Tập kèm server | `/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-0` |
| Tập kèm mã số | `/phim/bao-mau-bi-mat-cua-tieu-thu/tap-1-3097673` |
| Phim lẻ, có hậu tố ngôn ngữ | `/xem-phim/khu-rung-bi-tham/tap-full/vietsub` |

Link từng tập được dựng lại từ đúng `slug` mà host trả về nên không bị đoán sai thành `tap-N`; đoạn đánh dấu tập được nhận theo từng segment nên slug phim có sẵn số (`…-phan-3`) không bị hiểu nhầm là số tập. Tập được nhận theo **slug phim** thay vì theo prefix, nên trang liệt kê ở `/phim/<slug>` mà phát ở `/xem-phim/<slug>/…` vẫn ra đủ danh sách; phim lẻ được link hai lần (`tap-full` và `tap-1`) chỉ hiện một tập.

Không tải được: một số phim chỉ phát qua trình nhúng (ví dụ `embed1.streamc.xyz`) với playlist mã hóa riêng thay vì `.m3u8` — FFmpeg không đọc được loại này. Khi đó ứng dụng báo rõ tên trình nhúng và lý do trong nhật ký thay vì chỉ nói "không tìm thấy luồng"; hãy đổi server hoặc tìm nguồn khác. Ngoài họ Motchill, mọi trang có `.m3u8`, `.mpd` hoặc video trực tiếp đều dùng được, kể cả khi phải **Mở file HTML** / **Dán HTML**.

Muốn kiểm tra thật với các host này (cần mạng):

```bash
cd wails-app && MOTCHILL_LIVE_HOSTS="https://motphimchill.cc/phim/xieu-long-giang-sinh|https://motchill.credit/phim/xac-song-thanh-pho-chet-phan-3/tap-1-sv-0" go test -run TestLiveHostVariants -v
```

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
