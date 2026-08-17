import {useEffect, useMemo, useRef, useState} from 'react';
import './App.css';
import {
    AnalyzeHTML, AnalyzeSource, AppendLog, CancelDownload, CancelShutdown, ChooseFFmpeg, ClearSession,
    GetInitialState, GetSavedSession, InstallFFmpeg, NewLogSession, PauseDownload, OpenHTMLFile, RevealFile,
    SaveLog, SaveSession, ScheduleShutdown, SelectOutputDirectory, StartDownload,
} from '../wailsjs/go/main/App';
import {EventsOff, EventsOn} from '../wailsjs/runtime/runtime';
import {HelpDialog, ResultsPanel, RestoreDialog, ShutdownBanner} from './panels';
import {
    episodeKey, errorMessage, folderName, statusLabels,
    type Analysis, type DoneEvent, type MovieEntry, type ProgressEvent, type QueueEvent,
    type SessionSummary, type Stream,
} from './types';

// Seconds between the queue finishing and the machine powering off.
const shutdownDelaySeconds = 60;

function App() {
    const [source, setSource] = useState('');
    const [movies, setMovies] = useState<MovieEntry[]>([]);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
    const [defaultOutputDir, setDefaultOutputDir] = useState('');
    const [preferredServer, setPreferredServer] = useState('');
    const [skipExisting, setSkipExisting] = useState(true);
    const [ffmpegReady, setFfmpegReady] = useState(false);
    const [ffmpegPath, setFfmpegPath] = useState('');
    const [platform, setPlatform] = useState('windows');
    const [appVersion, setAppVersion] = useState('');
    const [ffmpegInstalling, setFfmpegInstalling] = useState(false);
    const [ffmpegProgress, setFfmpegProgress] = useState(0);
    const [analyzing, setAnalyzing] = useState(false);
    const [analyzeProgress, setAnalyzeProgress] = useState('');
    const [running, setRunning] = useState(false);
    const [paused, setPaused] = useState(false);
    const [pauseBusy, setPauseBusy] = useState(false);
    const [notice, setNotice] = useState('Có thể nhập nhiều URL, mỗi URL một dòng.');
    const [error, setError] = useState('');
    const [queue, setQueue] = useState<Record<string, QueueEvent>>({});
    const [activeQueue, setActiveQueue] = useState<QueueEvent | null>(null);
    const [progress, setProgress] = useState<ProgressEvent | null>(null);
    const [done, setDone] = useState<DoneEvent | null>(null);
    const [logs, setLogs] = useState<string[]>([]);
    const [lastOutput, setLastOutput] = useState('');
    const [savedLogPath, setSavedLogPath] = useState('');
    const [autoLogPath, setAutoLogPath] = useState('');
    const [autoLogError, setAutoLogError] = useState('');
    const [logDir, setLogDir] = useState('');
    const [buildDate, setBuildDate] = useState('');
    const [canShutdown, setCanShutdown] = useState(false);
    // Per-episode folder overrides, keyed like the selection set.
    const [episodeDirs, setEpisodeDirs] = useState<Record<string, string>>({});
    const [shutdownWhenDone, setShutdownWhenDone] = useState(false);
    const [shutdownSeconds, setShutdownSeconds] = useState(0);
    const [shutdownSurvives, setShutdownSurvives] = useState(true);
    const [restore, setRestore] = useState<{summary: SessionSummary; state: any} | null>(null);
    const [helpOpen, setHelpOpen] = useState(false);
    const [sidePanel, setSidePanel] = useState<'log' | 'results'>('log');
    const [onlyFailed, setOnlyFailed] = useState(false);
    const movieSequence = useRef(0);
    const logEnd = useRef<HTMLDivElement>(null);
    // Every disk write goes through one promise chain so lines land in order.
    const logWrites = useRef<Promise<unknown>>(Promise.resolve());
    // The session is written on a short delay so typing or ticking many boxes
    // does not hit the disk once per keystroke.
    const sessionTimer = useRef<number | null>(null);
    const restoring = useRef(false);

    const allEpisodeKeys = useMemo(() => movies.flatMap(movie =>
        movie.analysis.episodes.map(episode => episodeKey(movie.key, episode.id))), [movies]);
    const selectedMovies = useMemo(() => movies.filter(movie =>
        movie.analysis.episodes.some(episode => selected.has(episodeKey(movie.key, episode.id)))), [movies, selected]);
    // Servers come from the current page and from every episode of the index,
    // so the picker lists all of them, not just the one that page opened with.
    const availableServers = useMemo(() => {
        const unique = new Map<string, Stream>();
        const collect = (stream: Stream) => {
            const key = stream.server || stream.kind;
            if (key && !unique.has(key)) unique.set(key, stream);
        };
        movies.forEach(movie => {
            (movie.analysis.streams ?? []).forEach(collect);
            movie.analysis.episodes.forEach(episode => (episode.streams ?? []).forEach(collect));
        });
        return [...unique.values()];
    }, [movies]);

    const selectedCount = selected.size;
    const allSelected = allEpisodeKeys.length > 0 && selectedCount === allEpisodeKeys.length;
    const allCollapsed = movies.length > 0 && movies.every(movie => collapsed.has(movie.key));
    const selectedDirectories = [...new Set(selectedMovies.map(movie => movie.outputDir).filter(Boolean))];
    const folderDisplay = selectedMovies.length === 0
        ? defaultOutputDir
        : selectedDirectories.length === 1 && selectedMovies.every(movie => movie.outputDir)
            ? selectedDirectories[0]
            : selectedDirectories.length > 1 ? `${selectedDirectories.length} thư mục khác nhau` : 'Có phim chưa chọn thư mục';

    // A folder chosen for one episode wins over the folder of its movie.
    function directoryFor(movie: MovieEntry, key: string) {
        return (episodeDirs[key] || movie.outputDir || '').trim();
    }

    function appendLog(line: string) {
        const stamp = new Date().toLocaleTimeString('vi-VN', {hour12: false});
        const entry = `[${stamp}] ${line}`;
        setLogs(previous => [...previous.slice(-299), entry]);
        persistLog(entry);
    }

    // Auto-save: each line is written to disk right away, so nothing is lost if
    // the app closes before "Lưu log" is pressed.
    function persistLog(entry: string) {
        logWrites.current = logWrites.current
            .then(() => AppendLog(entry))
            .then(path => {
                if (path) setAutoLogPath(previous => (path === previous ? previous : path));
                setAutoLogError(previous => (previous ? '' : previous));
            })
            .catch(reason => setAutoLogError(errorMessage(reason)));
    }

    // The done handler is registered once, so it reads the wish through a ref.
    const shutdownWanted = useRef(false);
    useEffect(() => { shutdownWanted.current = shutdownWhenDone; }, [shutdownWhenDone]);

    function requestShutdown() {
        if (!shutdownWanted.current) return;
        ScheduleShutdown(shutdownDelaySeconds).then((status: any) => {
            setShutdownSeconds(status.seconds || shutdownDelaySeconds);
            setShutdownSurvives(Boolean(status.survivesAppExit));
            appendLog(`Đã hẹn tắt máy sau ${status.seconds || shutdownDelaySeconds} giây (${status.at || ''}).`);
        }).catch((reason: unknown) => {
            setError(errorMessage(reason));
            appendLog(`Không hẹn được tắt máy: ${errorMessage(reason)}`);
        });
    }

    function stopShutdown() {
        CancelShutdown().then(() => {
            setShutdownSeconds(0);
            setShutdownWhenDone(false);
            appendLog('Đã hủy hẹn tắt máy.');
            setNotice('Đã hủy hẹn tắt máy.');
        }).catch((reason: unknown) => setError(errorMessage(reason)));
    }

    useEffect(() => {
        if (shutdownSeconds <= 0) return;
        const ticker = window.setInterval(() => setShutdownSeconds(previous => Math.max(0, previous - 1)), 1000);
        return () => window.clearInterval(ticker);
    }, [shutdownSeconds > 0]);

    function sessionState(finished: boolean) {
        return {
            finished,
            movies: movies.map(movie => ({
                key: movie.key, title: movie.analysis.title, source: movie.source,
                pageUrl: movie.analysis.pageUrl, outputDir: movie.outputDir, collapsed: collapsed.has(movie.key),
                episodes: movie.analysis.episodes.map(episode => {
                    const key = episodeKey(movie.key, episode.id);
                    const state = queue[key];
                    return {
                        id: episode.id, name: episode.name, number: episode.number, pageUrl: episode.pageUrl,
                        streamUrl: episode.streamUrl || '', streams: episode.streams ?? [],
                        outputDir: episodeDirs[key] || '',
                        selected: selected.has(key),
                        status: state && state.status !== 'resolving' && state.status !== 'downloading' ? state.status : '',
                        message: state?.message || '', output: state?.output || '',
                    };
                }),
            })),
        };
    }

    function restoreSession(state: any) {
        restoring.current = true;
        const restoredMovies: MovieEntry[] = [];
        const restoredSelected = new Set<string>();
        const restoredCollapsed = new Set<string>();
        const restoredDirs: Record<string, string> = {};
        const restoredQueue: Record<string, QueueEvent> = {};
        (state?.movies ?? []).forEach((movie: any) => {
            const episodes = (movie.episodes ?? []).map((episode: any) => ({
                id: episode.id, name: episode.name, number: episode.number || 0,
                pageUrl: episode.pageUrl, streamUrl: episode.streamUrl || '',
                streams: episode.streams ?? [], current: false,
            }));
            restoredMovies.push({
                key: movie.key, source: movie.source || '', outputDir: movie.outputDir || '',
                analysis: {
                    title: movie.title || 'Phim', pageUrl: movie.pageUrl || '', streams: [],
                    episodes, htmlBytes: 0, sourceLabel: movie.source || '',
                },
            });
            if (movie.collapsed) restoredCollapsed.add(movie.key);
            (movie.episodes ?? []).forEach((episode: any) => {
                const key = episodeKey(movie.key, episode.id);
                if (episode.selected) restoredSelected.add(key);
                if (episode.outputDir) restoredDirs[key] = episode.outputDir;
                if (episode.status) {
                    restoredQueue[key] = {
                        id: key, movie: movie.title || 'Phim', name: episode.name, status: episode.status,
                        index: 0, total: 0, completed: 0, failed: 0, skipped: 0,
                        message: episode.message || '', output: episode.output || '',
                    };
                }
            });
        });
        setMovies(restoredMovies);
        setSelected(restoredSelected);
        setCollapsed(restoredCollapsed);
        setEpisodeDirs(restoredDirs);
        setQueue(restoredQueue);
        setSkipExisting(true);
        setRestore(null);
        setSidePanel('results');
        const episodeCount = restoredMovies.reduce((count, movie) => count + movie.analysis.episodes.length, 0);
        setNotice(`Đã mở lại danh sách: ${restoredMovies.length} phim · ${episodeCount} tập.`);
        appendLog(`Mở lại danh sách đã lưu: ${restoredMovies.length} phim, ${episodeCount} tập.`);
        restoring.current = false;
    }

    function discardSession() {
        setRestore(null);
        ClearSession().catch((reason: unknown) => setError(errorMessage(reason)));
        setNotice('Đã bỏ danh sách cũ. Nhập link mới để bắt đầu.');
    }

    function startLogSession() {
        logWrites.current = logWrites.current
            .then(() => NewLogSession())
            .catch(reason => setAutoLogError(errorMessage(reason)));
        setAutoLogPath('');
    }

    useEffect(() => {
        GetInitialState().then((state: any) => {
            setDefaultOutputDir(state.lastOutputDir || '');
            setFfmpegReady(Boolean(state.ffmpegReady));
            setFfmpegPath(state.ffmpegPath || '');
            setPlatform(state.platform || 'windows');
            setAppVersion(state.version || '');
            setLogDir(state.logDir || '');
            setBuildDate(state.buildDate || '');
            setCanShutdown(Boolean(state.canShutdown));
            if (state.logPath) setAutoLogPath(state.logPath);
        }).catch((reason: unknown) => setError(errorMessage(reason)));

        // Only ask about the previous list when it still has work left in it.
        GetSavedSession().then((saved: any) => {
            if (saved?.summary?.needsAttention) setRestore({summary: saved.summary, state: saved.state});
        }).catch(() => undefined);

        EventsOn('download:queue', (event: QueueEvent) => {
            setActiveQueue(event);
            setQueue(previous => ({...previous, [event.id || `${event.movie}:${event.name}`]: event}));
            const label = `${event.movie} · ${event.name}`;
            if (event.status !== 'downloading') setProgress(previous => (previous?.id === event.id ? null : previous));
            if (event.status === 'downloading' && (event.attempt ?? 1) <= 1) appendLog(`${label}: bắt đầu tải.`);
            if (event.status === 'completed') {
                if (event.output) setLastOutput(event.output);
                appendLog(`${label}: hoàn tất${event.output ? ` → ${event.output}` : '.'}`);
            }
            if (event.status === 'skipped') appendLog(`${label}: bỏ qua — ${event.message || 'file đã có'}.`);
            if (event.status === 'failed') appendLog(`${label}: LỖI — ${event.message || 'tải thất bại'}`);
        });
        EventsOn('download:progress', (event: ProgressEvent) => setProgress(event));
        EventsOn('download:log', (line: string) => appendLog(line));
        EventsOn('download:done', (event: DoneEvent) => {
            setDone(event); setRunning(false); setPaused(false); setProgress(null);
            appendLog(event.cancelled
                ? `Đã hủy hàng đợi: ${event.completed}/${event.total} tập hoàn tất.`
                : `Kết thúc: ${event.completed} thành công, ${event.failed} lỗi, ${event.skipped} bỏ qua.`);
            setNotice(event.cancelled
                ? `Đã hủy. Hoàn tất ${event.completed}/${event.total} tập.`
                : `Xong hàng đợi: ${event.completed} thành công, ${event.failed} lỗi, ${event.skipped} bỏ qua.`);
            setSidePanel('results');
            if (!event.cancelled) requestShutdown();
        });
        EventsOn('ffmpeg:progress', (event: {downloaded: number; total: number}) => {
            if (event.total > 0) setFfmpegProgress(Math.min(100, Math.round(event.downloaded * 100 / event.total)));
        });
        return () => {
            ['download:queue', 'download:progress', 'download:log', 'download:done', 'ffmpeg:progress'].forEach(name => EventsOff(name));
        };
    }, []);

    useEffect(() => { logEnd.current?.scrollIntoView({behavior: 'smooth'}); }, [logs]);

    // Keep the saved session in step with the list, its folders and its results.
    useEffect(() => {
        if (restoring.current || restore) return;
        if (sessionTimer.current) window.clearTimeout(sessionTimer.current);
        sessionTimer.current = window.setTimeout(() => {
            if (!movies.length) return;
            SaveSession(sessionState(!running && Boolean(done)) as any)
                .catch((reason: unknown) => setAutoLogError(errorMessage(reason)));
        }, 700);
        return () => { if (sessionTimer.current) window.clearTimeout(sessionTimer.current); };
    }, [movies, selected, collapsed, episodeDirs, queue, running, done, restore]);

    function addAnalysis(result: Analysis, originalSource: string) {
        const sortedEpisodes = [...(result.episodes ?? [])].sort((a, b) => (a.number || 999999) - (b.number || 999999));
        const key = `movie-${Date.now()}-${++movieSequence.current}`;
        const entry: MovieEntry = {
            key, source: originalSource, outputDir: defaultOutputDir,
            analysis: {...result, streams: result.streams ?? [], episodes: sortedEpisodes},
        };
        setMovies(previous => [...previous, entry]);
        setSelected(previous => {
            const next = new Set(previous);
            sortedEpisodes.forEach(episode => next.add(episodeKey(key, episode.id)));
            return next;
        });
        appendLog(`Đã thêm: ${result.title} — ${sortedEpisodes.length} tập.`);
    }

    async function analyze() {
        const links = (source.match(/https?:\/\/[^\s]+/g) ?? [])
            .map(link => link.replace(/[),;\]]+$/, ''));
        if (!links.length) { setError('Hãy nhập ít nhất một URL. Mỗi URL nên nằm trên một dòng.'); return; }
        setAnalyzing(true); setError('');
        let added = 0;
        const failures: string[] = [];
        for (let index = 0; index < links.length; index++) {
            const link = links[index];
            setAnalyzeProgress(`${index + 1}/${links.length}`);
            setNotice(`Đang phân tích ${index + 1}/${links.length}: ${link}`);
            try {
                addAnalysis(await AnalyzeSource(link) as Analysis, link);
                added++;
            } catch (reason) {
                failures.push(`${link}\n${errorMessage(reason)}`);
                // Analysis failures used to live only in the error box, which
                // left nothing in the log to look at afterwards.
                appendLog(`Không phân tích được ${link}: ${errorMessage(reason)}`);
            }
        }
        setAnalyzing(false); setAnalyzeProgress('');
        setSource(failures.length ? failures.map(value => value.split('\n')[0]).join('\n') : '');
        setNotice(`Đã thêm ${added}/${links.length} phim vào danh sách.`);
        if (failures.length) setError(`Không phân tích được ${failures.length} link:\n${failures.join('\n\n')}`);
    }

    async function openHTML() {
        setError('');
        try {
            const document = await OpenHTMLFile() as {path: string; content: string};
            if (!document?.content) return;
            setAnalyzing(true); setAnalyzeProgress('1/1');
            addAnalysis(await AnalyzeHTML(document.content) as Analysis, document.path);
            setNotice('Đã thêm phim từ file HTML vào danh sách.');
        } catch (reason) { setError(errorMessage(reason)); }
        finally { setAnalyzing(false); setAnalyzeProgress(''); }
    }

    async function pasteHTML() {
        setError('');
        try {
            const content = await navigator.clipboard.readText();
            if (!content.trim()) throw new Error('Clipboard không có văn bản.');
            setAnalyzing(true); setAnalyzeProgress('1/1');
            addAnalysis(await AnalyzeHTML(content) as Analysis, `HTML đã dán (${content.length.toLocaleString('vi-VN')} ký tự)`);
            setNotice('Đã thêm phim từ HTML vào danh sách.');
        } catch (reason) { setError(errorMessage(reason)); }
        finally { setAnalyzing(false); setAnalyzeProgress(''); }
    }

    async function selectDirectoryForSelectedMovies() {
        if (!selectedMovies.length) { setError('Hãy chọn ít nhất một phim trước khi chọn thư mục lưu.'); return; }
        try {
            const selectedPath = await SelectOutputDirectory();
            if (!selectedPath) return;
            const selectedKeys = new Set(selectedMovies.map(movie => movie.key));
            setMovies(previous => previous.map(movie => selectedKeys.has(movie.key) ? {...movie, outputDir: selectedPath} : movie));
            setDefaultOutputDir(selectedPath);
            setNotice(`Đã áp dụng thư mục cho ${selectedMovies.length} phim đang chọn.`);
        } catch (reason) { setError(errorMessage(reason)); }
    }

    // Folder for one movie, chosen straight from its row in the list.
    async function selectDirectoryForMovie(movie: MovieEntry) {
        try {
            const selectedPath = await SelectOutputDirectory();
            if (!selectedPath) return;
            setMovies(previous => previous.map(item => item.key === movie.key ? {...item, outputDir: selectedPath} : item));
            setNotice(`${movie.analysis.title} → ${selectedPath}`);
        } catch (reason) { setError(errorMessage(reason)); }
    }

    // Folder for a single episode; it overrides the folder of its movie.
    async function selectDirectoryForEpisode(key: string, episodeName: string) {
        try {
            const selectedPath = await SelectOutputDirectory();
            if (!selectedPath) return;
            setEpisodeDirs(previous => ({...previous, [key]: selectedPath}));
            setNotice(`${episodeName} → ${selectedPath}`);
        } catch (reason) { setError(errorMessage(reason)); }
    }

    function clearEpisodeDirectory(key: string) {
        setEpisodeDirs(previous => {
            const next = {...previous}; delete next[key]; return next;
        });
    }

    async function installFFmpeg() {
        setFfmpegInstalling(true); setFfmpegProgress(0); setError('');
        setNotice(platform === 'darwin' ? 'Đang cài FFmpeg bằng Homebrew...' : 'Đang tải FFmpeg Essentials...');
        try {
            const status: any = await InstallFFmpeg();
            setFfmpegReady(Boolean(status.ready)); setFfmpegPath(status.path || ''); setNotice('FFmpeg đã sẵn sàng.');
        } catch (reason) { setError(errorMessage(reason)); setNotice('Không thể cài FFmpeg.'); }
        finally { setFfmpegInstalling(false); }
    }

    async function chooseFFmpeg() {
        try {
            const status: any = await ChooseFFmpeg();
            setFfmpegReady(Boolean(status.ready)); setFfmpegPath(status.path || '');
        } catch (reason) { setError(errorMessage(reason)); }
    }

    function toggleAll() {
        setSelected(allSelected ? new Set() : new Set(allEpisodeKeys));
    }

    function toggleMovie(movie: MovieEntry) {
        const keys = movie.analysis.episodes.map(episode => episodeKey(movie.key, episode.id));
        const completelySelected = keys.length > 0 && keys.every(key => selected.has(key));
        setSelected(previous => {
            const next = new Set(previous);
            keys.forEach(key => completelySelected ? next.delete(key) : next.add(key));
            return next;
        });
    }

    function toggleCollapse(movieKey: string) {
        setCollapsed(previous => {
            const next = new Set(previous);
            next.has(movieKey) ? next.delete(movieKey) : next.add(movieKey);
            return next;
        });
    }

    function toggleCollapseAll() {
        setCollapsed(allCollapsed ? new Set() : new Set(movies.map(movie => movie.key)));
    }

    function toggleEpisode(key: string) {
        setSelected(previous => {
            const next = new Set(previous); next.has(key) ? next.delete(key) : next.add(key); return next;
        });
    }

    function removeMovie(movie: MovieEntry) {
        const keys = new Set(movie.analysis.episodes.map(episode => episodeKey(movie.key, episode.id)));
        setMovies(previous => previous.filter(item => item.key !== movie.key));
        setSelected(previous => new Set([...previous].filter(key => !keys.has(key))));
        setCollapsed(previous => {
            const next = new Set(previous); next.delete(movie.key); return next;
        });
        setEpisodeDirs(previous => Object.fromEntries(
            Object.entries(previous).filter(([key]) => !keys.has(key))));
    }

    function clearList() {
        setMovies([]); setSelected(new Set()); setCollapsed(new Set());
        setQueue({}); setEpisodeDirs({}); setDone(null);
        ClearSession().catch(() => undefined);
    }

    async function startDownload() {
        if (!ffmpegReady) { setError(`Chưa có FFmpeg. Hãy cài hoặc chọn ${platform === 'darwin' ? 'binary ffmpeg' : 'ffmpeg.exe'}.`); return; }
        const selectedEntries = movies.flatMap(movie => movie.analysis.episodes
            .filter(episode => selected.has(episodeKey(movie.key, episode.id)))
            .map(episode => ({movie, episode, key: episodeKey(movie.key, episode.id)})));
        if (!selectedEntries.length) { setError('Hãy chọn ít nhất một tập.'); return; }
        const missingFolders = [...new Set(selectedEntries
            .filter(item => !directoryFor(item.movie, item.key))
            .map(item => item.movie.analysis.title))];
        if (missingFolders.length) { setError(`Chưa chọn thư mục lưu cho: ${missingFolders.join(', ')}`); return; }
        setError(''); setQueue({}); setDone(null); setLogs([]); setSavedLogPath(''); setActiveQueue(null); setProgress(null);
        startLogSession();
        appendLog(`Bắt đầu hàng đợi: ${selectedEntries.length} tập của ${selectedMovies.length} phim.`);
        try {
            await StartDownload({
                title: 'Nhiều phim', outputDir: defaultOutputDir, preferredServer, skipExisting,
                items: selectedEntries.map(({movie, episode, key}) => ({
                    id: key, name: episode.name, number: episode.number, pageUrl: episode.pageUrl,
                    streamUrl: episode.streamUrl || '', streams: episode.streams ?? [],
                    title: movie.analysis.title, outputDir: directoryFor(movie, key),
                })),
            } as any);
            setRunning(true);
            setPaused(false);
            setNotice(`Đã xếp ${selectedEntries.length} tập của ${selectedMovies.length} phim vào hàng đợi tuần tự.`);
        } catch (reason) { setError(errorMessage(reason)); }
    }

    async function togglePause() {
        setPauseBusy(true); setError('');
        try {
            const status: any = await PauseDownload();
            setPaused(Boolean(status.paused));
            setNotice(status.paused ? 'Đã tạm dừng FFmpeg. Bấm Tiếp tục để tải tiếp.' : 'Đã tiếp tục hàng đợi tải.');
        } catch (reason) { setError(errorMessage(reason)); }
        finally { setPauseBusy(false); }
    }

    async function cancelDownload() { setPaused(false); await CancelDownload(); setNotice('Đang dừng FFmpeg...'); }

    async function saveLog() {
        if (!logs.length) { setError('Nhật ký đang trống.'); return; }
        const header = [
            `Video HTML Downloader v${appVersion || 'unknown'}`,
            `Thời điểm lưu: ${new Date().toLocaleString('vi-VN')}`,
            `Số phim trong danh sách: ${movies.length}`,
            `Số tập đã chọn: ${selectedCount}`,
            '------------------------------------------------------------',
        ];
        try {
            const path = await SaveLog([...header, ...logs].join('\n'));
            if (path) { setSavedLogPath(path); setNotice(`Đã lưu nhật ký: ${path}`); }
        } catch (reason) { setError(errorMessage(reason)); }
    }

    const resultCounts = useMemo(() => {
        const values = Object.values(queue);
        return {
            failed: values.filter(state => state.status === 'failed').length,
            completed: values.filter(state => state.status === 'completed').length,
        };
    }, [queue]);

    // The episode being downloaded contributes its own percentage, so the bar
    // keeps creeping instead of jumping once per episode.
    const activeFraction = progress && progress.percent >= 0 ? progress.percent / 100
        : activeQueue && ['completed', 'skipped'].includes(activeQueue.status) ? 1 : 0;
    const overallPercent = done && !running ? 100 : activeQueue
        ? Math.max(2, Math.min(100, Math.round(((activeQueue.index - 1 + activeFraction) / activeQueue.total) * 100))) : 0;

    return (
        <main className="shell">
            <header className="topbar">
                <div className="brand"><div className="brand-mark">V</div><div><h1>Video HTML Downloader {appVersion && <span className="version-badge">v{appVersion}</span>}</h1><p>Nhiều phim · nhiều thư mục · tải tuần tự{buildDate && ` · cập nhật ${buildDate}`}</p></div></div>
                <button className="help-button" onClick={() => setHelpOpen(true)} title="Hướng dẫn sử dụng, nguồn hỗ trợ, phiên bản">❔ Hướng dẫn</button>
                <div className={`ffmpeg-pill ${ffmpegReady ? 'ready' : ''}`} title={ffmpegPath || 'Chưa cấu hình'}><span className="status-dot"/> FFmpeg {ffmpegReady ? 'sẵn sàng' : 'chưa có'}</div>
            </header>

            <section className="source-card card">
                <div className="section-heading"><span className="step">01</span><div><h2>Nguồn video</h2><p>Dán nhiều URL, mỗi dòng một link. Kết quả mới sẽ được thêm vào danh sách hiện có.</p></div></div>
                <div className="source-row source-multi-row">
                    <textarea value={source} onChange={event => setSource(event.target.value)}
                              onKeyDown={event => event.ctrlKey && event.key === 'Enter' && !analyzing && analyze()}
                              placeholder={'https://example.com/phim/bo-1\nhttps://example.com/phim/bo-2'} disabled={analyzing || running} spellCheck={false}/>
                    <button className="primary" onClick={analyze} disabled={analyzing || running}>{analyzing ? <><span className="spinner"/> {analyzeProgress}</> : 'Phân tích & thêm'}</button>
                </div>
                <div className="source-actions"><button className="ghost" onClick={openHTML} disabled={analyzing || running}>Mở file HTML</button><button className="ghost" onClick={pasteHTML} disabled={analyzing || running}>Dán HTML</button><span className="source-note">Ctrl + Enter để phân tích nhanh.</span></div>
            </section>

            {error && <div className="alert multiline"><strong>Có lỗi:</strong> {error}<button onClick={() => setError('')}>×</button></div>}

            <div className="workspace-grid">
                <section className="card episode-card movie-library-card">
                    <div className="section-heading compact"><span className="step">02</span><div><h2>Danh sách phim</h2><p>{movies.length ? `${movies.length} phim · ${allEpisodeKeys.length} tập · đã chọn ${selectedCount}` : 'Chưa có dữ liệu phân tích'}</p></div></div>
                    {movies.length > 0 && <div className="selection-toolbar library-toolbar">
                        <label className={`check-action ${allSelected ? 'checked' : ''}`}><input type="checkbox" checked={allSelected} onChange={toggleAll} disabled={running}/><span className="custom-check">✓</span><b>Chọn tất cả</b></label>
                        <button onClick={() => setSelected(new Set())} disabled={running}>Bỏ chọn tất cả</button>
                        <button onClick={toggleCollapseAll}>{allCollapsed ? '▾ Mở rộng tất cả' : '▸ Thu gọn tất cả'}</button>
                        <button onClick={clearList} disabled={running}>Xóa danh sách</button>
                    </div>}
                    <div className={`episode-list movie-list ${movies.length === 0 ? 'empty' : ''}`}>
                        {movies.length === 0 && <div className="empty-state"><div className="empty-icon">⌁</div><strong>Chưa có phim trong danh sách</strong><span>Nhập một hoặc nhiều link ở bước 01.</span></div>}
                        {movies.map(movie => {
                            const movieKeys = movie.analysis.episodes.map(episode => episodeKey(movie.key, episode.id));
                            const movieSelected = movieKeys.filter(key => selected.has(key)).length;
                            const movieAllSelected = movieKeys.length > 0 && movieSelected === movieKeys.length;
                            const movieStates = movieKeys.map(key => queue[key]?.status).filter(Boolean) as string[];
                            const movieDone = movieStates.filter(status => status === 'completed').length;
                            const movieSkipped = movieStates.filter(status => status === 'skipped').length;
                            const movieFailed = movieStates.filter(status => status === 'failed').length;
                            const isCollapsed = collapsed.has(movie.key);
                            const movieProgress = progress && progress.id.startsWith(`${movie.key}::`) ? progress : null;
                            return <article className={`movie-group ${isCollapsed ? 'collapsed' : ''}`} key={movie.key}>
                                <div className="movie-header">
                                    <button className="collapse-movie" onClick={() => toggleCollapse(movie.key)} title={isCollapsed ? 'Mở rộng danh sách tập' : 'Thu gọn danh sách tập'} aria-expanded={!isCollapsed}><span className="chevron">▾</span></button>
                                    <label className={`movie-check ${movieSelected ? 'checked' : ''}`}>
                                        <input type="checkbox" checked={movieAllSelected} onChange={() => toggleMovie(movie)} disabled={running}/><span className="custom-check">✓</span>
                                    </label>
                                    <div className="movie-meta" onClick={() => toggleCollapse(movie.key)} title={isCollapsed ? 'Bấm để mở rộng' : 'Bấm để thu gọn'}>
                                        <strong>{movie.analysis.title}</strong>
                                        <span>{movie.analysis.episodes.length} tập · đã chọn {movieSelected}{movieStates.length > 0 && <em className="movie-progress">✓ {movieDone} · ↷ {movieSkipped} · ! {movieFailed}</em>}</span>
                                        <small title={movie.outputDir || 'Chưa chọn thư mục'}>📁 {movie.outputDir || 'Chưa chọn thư mục lưu'}</small>
                                    </div>
                                    <button className="folder-movie" onClick={() => selectDirectoryForMovie(movie)} disabled={running} title="Chọn thư mục lưu cho cả phim này">📁</button>
                                    <button className="remove-movie" onClick={() => removeMovie(movie)} disabled={running} title="Xóa phim khỏi danh sách">×</button>
                                    {isCollapsed && movieProgress && <div className="movie-header-progress" title={`${movieProgress.name} · ${movieProgress.time}${movieProgress.duration ? ` / ${movieProgress.duration}` : ''}`}>
                                        <span className={`episode-bar ${movieProgress.percent < 0 ? 'unknown' : ''}`}><i style={movieProgress.percent >= 0 ? {width: `${movieProgress.percent}%`} : undefined}/></span>
                                        <b>{movieProgress.percent >= 0 ? `${Math.round(movieProgress.percent)}%` : '…'}</b>
                                    </div>}
                                </div>
                                <div className="movie-episodes" style={{maxHeight: isCollapsed ? 0 : movie.analysis.episodes.length * 52 + 24}}>
                                    {movie.analysis.episodes.map(episode => {
                                        const key = episodeKey(movie.key, episode.id);
                                        const state = queue[key];
                                        const live = progress && progress.id === key && state?.status === 'downloading' ? progress : null;
                                        return <label className={`episode-row ${selected.has(key) ? 'selected' : ''} ${state ? `status-${state.status}` : ''}`} key={key}>
                                            <input type="checkbox" checked={selected.has(key)} onChange={() => toggleEpisode(key)} disabled={running}/><span className="custom-check">✓</span>
                                            <span className="episode-number">{episode.number ? String(episode.number).padStart(2, '0') : '—'}</span>
                                            <span className="episode-name">{episode.name}{episode.current && <em>đang xem</em>}
                                                {episodeDirs[key] && <b className="episode-dir" title={`Thư mục riêng: ${episodeDirs[key]}`}>📁 {folderName(episodeDirs[key])}
                                                    <i onClick={event => { event.preventDefault(); event.stopPropagation(); clearEpisodeDirectory(key); }} title="Dùng lại thư mục của phim">×</i>
                                                </b>}
                                            </span>
                                            <button className="folder-episode" disabled={running}
                                                    onClick={event => { event.preventDefault(); event.stopPropagation(); selectDirectoryForEpisode(key, episode.name); }}
                                                    title="Chọn thư mục lưu riêng cho tập này">📁</button>
                                            {state && <span className={`queue-status ${state.status}`}>{statusLabels[state.status]}{state.status === 'downloading' && (state.attempt ?? 1) > 1 ? ` · lần ${state.attempt}` : ''}</span>}
                                            {live && <span className="episode-progress">
                                                <span className={`episode-bar ${live.percent < 0 ? 'unknown' : ''}`}><i style={live.percent >= 0 ? {width: `${live.percent}%`} : undefined}/></span>
                                                <b>{live.percent >= 0 ? `${live.percent.toFixed(1)}%` : 'Đang tải'}</b>
                                                <em>{live.time || '0:00'}{live.duration ? ` / ${live.duration}` : ''}{live.speed ? ` · ${live.speed}` : ''}</em>
                                            </span>}
                                        </label>;
                                    })}
                                </div>
                            </article>;
                        })}
                    </div>
                </section>

                <aside className="side-column">
                    <section className="card settings-card">
                        <div className="section-heading compact"><span className="step">03</span><div><h2>Thiết lập tải</h2><p>Áp dụng cho các phim đang được chọn.</p></div></div>
                        <label className="field-label">Thư mục lưu <span>{selectedMovies.length} phim đang chọn</span></label>
                        <div className="path-row"><input value={folderDisplay} readOnly title={folderDisplay}/><button onClick={selectDirectoryForSelectedMovies} disabled={running || selectedMovies.length === 0}>Chọn</button></div>
                        <label className="field-label">Server ưu tiên</label>
                        <select value={preferredServer} onChange={event => setPreferredServer(event.target.value)} disabled={!movies.length || running}>
                            <option value="">Tự động — thử lần lượt khi lỗi</option>
                            {availableServers.map((stream, index) => <option value={stream.server} key={`${stream.url}-${index}`}>{stream.server || `Server ${index + 1}`} · {stream.kind}</option>)}
                        </select>
                        <label className="switch-row"><input type="checkbox" checked={skipExisting} onChange={event => setSkipExisting(event.target.checked)} disabled={running}/><span className="switch"><i/></span><span><strong>Bỏ qua file đã có</strong><small>Tiện tiếp tục một hàng đợi đang tải dở</small></span></label>
                        {canShutdown && <label className="switch-row"><input type="checkbox" checked={shutdownWhenDone} onChange={event => setShutdownWhenDone(event.target.checked)}/><span className="switch"><i/></span><span><strong>Tắt máy khi tải xong</strong><small>Chờ {shutdownDelaySeconds} giây sau khi hàng đợi kết thúc, có thể hủy</small></span></label>}
                        {shutdownSeconds > 0 && <ShutdownBanner seconds={shutdownSeconds} survivesAppExit={shutdownSurvives} onCancel={stopShutdown}/>}
                        {!ffmpegReady && <div className="ffmpeg-setup"><div><strong>Cần FFmpeg</strong><small>{platform === 'darwin' ? 'Cài qua Homebrew hoặc chọn binary ffmpeg có sẵn.' : 'Tải Essentials khoảng 106 MB hoặc chọn bản có sẵn.'}</small></div>{ffmpegInstalling && platform !== 'darwin' && <div className="mini-progress"><i style={{width: `${ffmpegProgress}%`}}/></div>}<div className="inline-actions"><button onClick={installFFmpeg} disabled={ffmpegInstalling}>{ffmpegInstalling ? (platform === 'darwin' ? 'Đang cài…' : `${ffmpegProgress}%`) : (platform === 'darwin' ? 'Cài bằng Homebrew' : 'Tự cài')}</button><button onClick={chooseFFmpeg} disabled={ffmpegInstalling}>Chọn file</button></div></div>}
                        {movies.length > 0 && <div className={`batch-summary ${allSelected ? 'all' : ''}`}><span>{selectedMovies.length} phim</span><strong>{selectedCount}/{allEpisodeKeys.length} tập</strong></div>}
                        {!running && <button className="download-button" onClick={startDownload} disabled={selectedCount === 0}>{`Tải ${selectedCount || ''} tập · ${selectedMovies.length} phim`}</button>}
                        {running && <div className="download-controls"><button className="pause-button" onClick={togglePause} disabled={pauseBusy || (!paused && activeQueue?.status !== 'downloading')}>{pauseBusy ? 'Đang xử lý…' : paused ? '▶ Tiếp tục' : '⏸ Tạm dừng'}</button><button className="stop-button" onClick={cancelDownload}>■ Dừng</button></div>}
                        <p className="legal-note">Chỉ tải nội dung bạn sở hữu hoặc được phép lưu. Không hỗ trợ DRM.</p>
                    </section>

                    <section className="card progress-card">
                        <div className="progress-head"><div><span>TRẠNG THÁI</span><strong>{notice}</strong></div>{running && activeQueue && <b>{activeQueue.index}/{activeQueue.total}</b>}</div>
                        <div className={`overall-progress ${running && !paused ? 'active' : ''} ${paused ? 'paused' : ''}`}><i style={{width: `${overallPercent}%`}}/></div>
                        {progress && running && <div className="live-stats"><span>{progress.name}</span><span>{progress.time || '0:00'}{progress.duration ? ` / ${progress.duration}` : ''}</span><span>{progress.percent >= 0 ? `${progress.percent.toFixed(1)}%` : '—'}</span><span>{progress.speed || '—'}</span></div>}
                        {done && !running && <div className="summary-chips"><span className="ok">✓ {done.completed}</span><span className="skip">↷ {done.skipped}</span><span className="bad">! {done.failed}</span>{lastOutput && done.completed > 0 && <button onClick={() => RevealFile(lastOutput)}>Mở thư mục</button>}</div>}
                        <div className="panel-tabs">
                            <button className={sidePanel === 'log' ? 'active' : ''} onClick={() => setSidePanel('log')}>Nhật ký</button>
                            <button className={sidePanel === 'results' ? 'active' : ''} onClick={() => setSidePanel('results')}>
                                Kết quả{resultCounts.failed > 0 && <b className="tab-badge">{resultCounts.failed}</b>}
                            </button>
                        </div>
                        {sidePanel === 'log' ? <>
                            <div className="log-actions">
                                <span className={autoLogError ? 'log-warning' : ''} title={autoLogError || autoLogPath || logDir}>
                                    {autoLogError ? `⚠ Không tự lưu được log: ${autoLogError}` : autoLogPath ? `✓ Tự động lưu · ${logs.length} dòng` : `${logs.length} dòng nhật ký`}
                                </span>
                                <button onClick={saveLog} disabled={!logs.length}>💾 Lưu bản sao</button>
                                {(autoLogPath || logDir) && <button onClick={() => RevealFile(autoLogPath || logDir)} title={autoLogPath || logDir}>📂 Thư mục log</button>}
                                {savedLogPath && <button onClick={() => RevealFile(savedLogPath)}>Mở bản sao</button>}
                            </div>
                            <div className="log-view">{logs.length === 0 ? <span>Nhật ký tải sẽ xuất hiện tại đây.</span> : logs.map((line, index) => <div key={index}>{line}</div>)}<div ref={logEnd}/></div>
                        </> : <ResultsPanel movies={movies} queue={queue} selected={selected} onlyFailed={onlyFailed}
                                           onToggleOnlyFailed={() => setOnlyFailed(value => !value)}
                                           onSelectFailed={keys => {
                                               setSelected(new Set(keys));
                                               setNotice(`Đã chọn ${keys.length} tập lỗi để tải lại.`);
                                           }}
                                           onReveal={path => RevealFile(path)}/>}
                    </section>
                </aside>
            </div>

            {restore && <RestoreDialog summary={restore.summary} onRestore={() => restoreSession(restore.state)} onDiscard={discardSession}/>}
            {helpOpen && <HelpDialog version={appVersion} buildDate={buildDate} logDir={logDir} platform={platform} onClose={() => setHelpOpen(false)}/>}
        </main>
    );
}

export default App;
