export type Stream = { url: string; kind: string; server: string };
export type Episode = {
    id: string; name: string; number: number; pageUrl: string;
    streamUrl?: string; streams?: Stream[] | null; current: boolean;
};
export type Analysis = {
    title: string; pageUrl: string; streams?: Stream[] | null; episodes: Episode[];
    htmlBytes: number; sourceLabel: string;
};
export type MovieEntry = { key: string; source: string; analysis: Analysis; outputDir: string };
export type QueueStatus = 'resolving' | 'downloading' | 'completed' | 'failed' | 'skipped';
export type QueueEvent = {
    id: string; movie: string; index: number; total: number; name: string; status: QueueStatus;
    output?: string; message?: string; completed: number; failed: number; skipped: number; attempt?: number;
};
export type ProgressEvent = {
    id: string; index: number; total: number; name: string;
    time: string; duration?: string; speed: string; percent: number;
};
export type DoneEvent = { total: number; completed: number; failed: number; skipped: number; cancelled: boolean };
export type SessionSummary = {
    movies: number; episodes: number; completed: number; failed: number; skipped: number;
    pending: number; needsAttention: boolean; savedAt: string; version: string;
};

export const statusLabels: Record<string, string> = {
    resolving: 'Đang lấy link', downloading: 'Đang tải', completed: 'Hoàn tất', failed: 'Lỗi', skipped: 'Đã có',
};

export function episodeKey(movieKey: string, episodeID: string) {
    return `${movieKey}::${episodeID}`;
}

export function errorMessage(error: unknown) {
    if (typeof error === 'string') return error;
    if (error instanceof Error) return error.message;
    return 'Đã xảy ra lỗi không xác định.';
}

// Folder paths are long; the list only needs the last segment to stay readable.
export function folderName(path: string) {
    const parts = path.split(/[\\/]/).filter(Boolean);
    return parts.length ? parts[parts.length - 1] : path;
}

export function formatSavedAt(value: string) {
    if (!value) return '';
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('vi-VN');
}
