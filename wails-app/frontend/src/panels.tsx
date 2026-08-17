import type {ReactNode} from 'react';
import {
    episodeKey, folderName, formatSavedAt, statusLabels,
    type MovieEntry, type QueueEvent, type SessionSummary,
} from './types';

export function Dialog({title, subtitle, onClose, children, actions}: {
    title: string; subtitle?: string; onClose?: () => void; children: ReactNode; actions?: ReactNode;
}) {
    return <div className="dialog-backdrop" onClick={event => event.target === event.currentTarget && onClose?.()}>
        <div className="dialog" role="dialog" aria-modal="true" aria-label={title}>
            <div className="dialog-head">
                <div><h3>{title}</h3>{subtitle && <p>{subtitle}</p>}</div>
                {onClose && <button className="dialog-close" onClick={onClose} title="Đóng">×</button>}
            </div>
            <div className="dialog-body">{children}</div>
            {actions && <div className="dialog-actions">{actions}</div>}
        </div>
    </div>;
}

/** Asked at startup when the previous list still had failed or unfinished episodes. */
export function RestoreDialog({summary, onRestore, onDiscard}: {
    summary: SessionSummary; onRestore: () => void; onDiscard: () => void;
}) {
    return <Dialog
        title="Mở lại danh sách lần trước?"
        subtitle={`Lưu lúc ${formatSavedAt(summary.savedAt)}${summary.version ? ` · v${summary.version}` : ''}`}
        actions={<>
            <button className="ghost" onClick={onDiscard}>Bắt đầu danh sách mới</button>
            <button className="primary" onClick={onRestore}>Mở lại danh sách</button>
        </>}>
        <p className="dialog-lead">Lần tải trước <strong>chưa hoàn thành</strong>. Danh sách đã lưu gồm:</p>
        <div className="restore-grid">
            <span><b>{summary.movies}</b>phim</span>
            <span><b>{summary.episodes}</b>tập trong danh sách</span>
            <span className="ok"><b>{summary.completed}</b>đã tải xong</span>
            <span className="bad"><b>{summary.failed}</b>lỗi</span>
            <span className="wait"><b>{summary.pending}</b>chưa tải</span>
            <span className="skip"><b>{summary.skipped}</b>đã có sẵn</span>
        </div>
        <p className="dialog-note">
            Mở lại sẽ giữ nguyên thư mục lưu của từng phim, từng tập và kết quả cũ, kèm bật
            <strong> Bỏ qua file đã có</strong> để không tải lại các tập đã hoàn tất.
        </p>
    </Dialog>;
}

const supportedSources = [
    ['Motchill và các biến thể', 'motchill.credit, motphimchill.cc, motphimchilll.me, phimmoichill.hair… — cùng một mã nguồn nên chỉ khác dạng link. Hỗ trợ cả /phim/… và /xem-phim/…, tập dạng tap-1, tap-1-sv-0, tap-1-3097673 hay tap-full/vietsub.'],
    ['Danh sách tập tự động', 'Đọc `episodeVariants` và API `/baseapi/episodes` để lấy trọn bộ tập từ một link duy nhất, dùng đúng slug của host nên link từng tập không bị đoán sai.'],
    ['HLS — .m3u8', 'Định dạng phổ biến nhất của các host phim; FFmpeg ghép segment thành một file MP4.'],
    ['DASH — .mpd', 'Hỗ trợ khi luồng không bị mã hóa DRM.'],
    ['Video trực tiếp', 'Link .mp4, .m4v, .webm, .mov, .mkv nằm trong trang hoặc dán trực tiếp.'],
    ['File / mã HTML', 'Dùng **Mở file HTML** hoặc **Dán HTML** khi trang chặn tải tự động hoặc cần cookie đăng nhập.'],
];

export function HelpDialog({version, buildDate, logDir, platform, onClose}: {
    version: string; buildDate: string; logDir: string; platform: string; onClose: () => void;
}) {
    return <Dialog title="Hướng dẫn sử dụng" subtitle={`Video HTML Downloader v${version || '—'} · cập nhật ${buildDate || '—'}`}
                   onClose={onClose} actions={<button className="primary" onClick={onClose}>Đã hiểu</button>}>
        <ol className="help-steps">
            <li><b>Dán link</b> — mỗi dòng một URL trang phim hoặc trang tập, rồi bấm <b>Phân tích & thêm</b> (Ctrl + Enter). Phim mới được thêm vào danh sách đang có.</li>
            <li><b>Chọn tập</b> — tick từng tập, từng phim, hoặc <b>Chọn tất cả</b>. Bấm mũi tên đầu dòng để thu gọn từng nhóm phim.</li>
            <li><b>Chọn thư mục lưu</b> — nút 📁 ở dòng phim áp cho cả phim, nút 📁 ở dòng tập chỉ áp cho tập đó và được ưu tiên.</li>
            <li><b>Tải</b> — lần đầu cần cài hoặc chọn FFmpeg. Hàng đợi chạy tuần tự, mỗi tập có thanh tiến độ riêng; có thể Tạm dừng, Dừng, hoặc bật <b>Tắt máy khi tải xong</b>.</li>
            <li><b>Xem kết quả</b> — thẻ <b>Kết quả</b> liệt kê tập nào xong, lỗi (kèm lý do) hay chưa tải, và cho chọn lại đúng các tập lỗi để tải lại.</li>
        </ol>
        <h4>Nguồn được hỗ trợ</h4>
        <ul className="help-list">
            {supportedSources.map(([name, note]) => <li key={name}><b>{name}</b><span>{note.replace(/[`*]/g, '')}</span></li>)}
        </ul>
        <h4>Chống treo &amp; nhật ký</h4>
        <ul className="help-list">
            <li><b>Nhiều server mỗi tập</b><span>Mỗi tập giữ link của tất cả server; server 404 sẽ tự chuyển sang server khác, hết server thì lấy link mới từ trang tập.</span></li>
            <li><b>Tự tải lại</b><span>Nếu FFmpeg đứng yên 90 giây, tiến trình bị tắt và tập đó được tải lại tối đa 3 lần trước khi thử server khác.</span></li>
            <li><b>Không còn tiến trình mồ côi</b><span>FFmpeg bị hệ điều hành tắt cùng ứng dụng, kể cả khi ứng dụng bị tắt cứng.</span></li>
            <li><b>Tiến độ ngoài cửa sổ</b><span>Phần trăm hiện trên tiêu đề và trên thanh taskbar (vàng khi tạm dừng, đỏ khi có tập lỗi); tải xong sẽ có thông báo Toast.</span></li>
            <li><b>Log tự lưu</b><span>{logDir || 'Thư mục cấu hình của ứng dụng'}</span></li>
            <li><b>Danh sách được ghi nhớ</b><span>Nếu còn tập lỗi hoặc chưa tải, ứng dụng sẽ hỏi mở lại danh sách ở lần mở kế tiếp.</span></li>
        </ul>
        <p className="dialog-note">
            Nền tảng: {platform === 'darwin' ? 'macOS' : platform === 'windows' ? 'Windows' : platform}. Ứng dụng không phá DRM —
            chỉ tải nội dung bạn sở hữu hoặc được phép lưu.
        </p>
    </Dialog>;
}

const resultIcons: Record<string, string> = {completed: '✓', failed: '!', skipped: '↷', downloading: '⟳', resolving: '⟳'};

/** Per-movie breakdown of the current run: done, failed with reason, not started. */
export function ResultsPanel({movies, queue, selected, onlyFailed, onToggleOnlyFailed, onSelectFailed, onReveal}: {
    movies: MovieEntry[];
    queue: Record<string, QueueEvent>;
    selected: Set<string>;
    onlyFailed: boolean;
    onToggleOnlyFailed: () => void;
    onSelectFailed: (keys: string[]) => void;
    onReveal: (path: string) => void;
}) {
    const groups = movies.map(movie => {
        const rows = movie.analysis.episodes.map(episode => {
            const key = episodeKey(movie.key, episode.id);
            return {key, episode, state: queue[key], pending: !queue[key] && selected.has(key)};
        });
        return {
            movie, rows,
            completed: rows.filter(row => row.state?.status === 'completed').length,
            failed: rows.filter(row => row.state?.status === 'failed'),
            skipped: rows.filter(row => row.state?.status === 'skipped').length,
            pending: rows.filter(row => row.pending).length,
        };
    });
    const failedKeys = groups.flatMap(group => group.failed.map(row => row.key));

    if (!movies.length) return <div className="results-empty">Chưa có phim nào trong danh sách.</div>;

    return <div className="results-panel">
        <div className="results-toolbar">
            <label className={`mini-check ${onlyFailed ? 'checked' : ''}`}>
                <input type="checkbox" checked={onlyFailed} onChange={onToggleOnlyFailed}/>
                <span className="custom-check">✓</span>Chỉ hiện lỗi &amp; chưa xong
            </label>
            <button disabled={!failedKeys.length} onClick={() => onSelectFailed(failedKeys)}>
                Chọn {failedKeys.length || ''} tập lỗi
            </button>
        </div>
        <div className="results-list">
            {groups.map(group => {
                const rows = onlyFailed
                    ? group.rows.filter(row => row.state?.status === 'failed' || row.pending)
                    : group.rows.filter(row => row.state || row.pending);
                if (!rows.length) return null;
                return <section className="results-group" key={group.movie.key}>
                    <header>
                        <strong title={group.movie.analysis.title}>{group.movie.analysis.title}</strong>
                        <span>
                            <b className="ok">✓ {group.completed}</b>
                            <b className="bad">! {group.failed.length}</b>
                            <b className="skip">↷ {group.skipped}</b>
                            <b className="wait">⧗ {group.pending}</b>
                        </span>
                    </header>
                    {rows.map(row => <div className={`results-row ${row.state?.status ?? 'pending'}`} key={row.key}>
                        <i>{row.state ? resultIcons[row.state.status] ?? '·' : '⧗'}</i>
                        <span className="results-name">{row.episode.name}</span>
                        <span className="results-note" title={row.state?.message || row.state?.output || ''}>
                            {row.state?.status === 'failed' ? row.state.message || 'Tải thất bại'
                                : row.state?.status === 'completed' ? folderName(row.state.output || '') || 'Đã lưu'
                                    : row.state ? statusLabels[row.state.status] : 'Chưa tải'}
                        </span>
                        {row.state?.status === 'completed' && row.state.output &&
                            <button onClick={() => onReveal(row.state!.output!)} title={row.state.output}>Mở</button>}
                    </div>)}
                </section>;
            })}
        </div>
    </div>;
}

export function ShutdownBanner({seconds, survivesAppExit, onCancel}: {
    seconds: number; survivesAppExit: boolean; onCancel: () => void;
}) {
    const minutes = Math.floor(seconds / 60);
    const remainder = seconds % 60;
    return <div className="shutdown-banner">
        <span className="shutdown-dot"/>
        <div>
            <strong>Máy sẽ tắt sau {minutes > 0 ? `${minutes} phút ` : ''}{remainder} giây</strong>
            <small>{survivesAppExit ? 'Hệ điều hành giữ hẹn giờ này, đóng ứng dụng vẫn tắt máy.' : 'Đóng ứng dụng sẽ hủy hẹn tắt máy.'}</small>
        </div>
        <button onClick={onCancel}>Hủy tắt máy</button>
    </div>;
}
