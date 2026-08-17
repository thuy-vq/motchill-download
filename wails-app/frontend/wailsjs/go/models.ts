export namespace main {
	
	export class Episode {
	    id: string;
	    name: string;
	    number: number;
	    pageUrl: string;
	    streamUrl?: string;
	    streams?: MediaStream[];
	    current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Episode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.number = source["number"];
	        this.pageUrl = source["pageUrl"];
	        this.streamUrl = source["streamUrl"];
	        this.streams = this.convertValues(source["streams"], MediaStream);
	        this.current = source["current"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MediaStream {
	    url: string;
	    kind: string;
	    server: string;
	
	    static createFrom(source: any = {}) {
	        return new MediaStream(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.kind = source["kind"];
	        this.server = source["server"];
	    }
	}
	export class AnalysisResult {
	    title: string;
	    pageUrl: string;
	    streams: MediaStream[];
	    episodes: Episode[];
	    htmlBytes: number;
	    sourceLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new AnalysisResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.pageUrl = source["pageUrl"];
	        this.streams = this.convertValues(source["streams"], MediaStream);
	        this.episodes = this.convertValues(source["episodes"], Episode);
	        this.htmlBytes = source["htmlBytes"];
	        this.sourceLabel = source["sourceLabel"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DownloadControlStatus {
	    paused: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DownloadControlStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.paused = source["paused"];
	    }
	}
	export class DownloadItem {
	    id: string;
	    name: string;
	    number: number;
	    pageUrl: string;
	    streamUrl?: string;
	    streams?: MediaStream[];
	    title?: string;
	    outputDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.number = source["number"];
	        this.pageUrl = source["pageUrl"];
	        this.streamUrl = source["streamUrl"];
	        this.streams = this.convertValues(source["streams"], MediaStream);
	        this.title = source["title"];
	        this.outputDir = source["outputDir"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DownloadRequest {
	    title: string;
	    outputDir: string;
	    preferredServer: string;
	    items: DownloadItem[];
	    skipExisting: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DownloadRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.outputDir = source["outputDir"];
	        this.preferredServer = source["preferredServer"];
	        this.items = this.convertValues(source["items"], DownloadItem);
	        this.skipExisting = source["skipExisting"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class FFmpegStatus {
	    ready: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new FFmpegStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.path = source["path"];
	    }
	}
	export class InitialState {
	    lastOutputDir: string;
	    ffmpegReady: boolean;
	    ffmpegPath: string;
	    platform: string;
	    version: string;
	    buildDate: string;
	    logDir: string;
	    logPath: string;
	    canShutdown: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InitialState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lastOutputDir = source["lastOutputDir"];
	        this.ffmpegReady = source["ffmpegReady"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.platform = source["platform"];
	        this.version = source["version"];
	        this.buildDate = source["buildDate"];
	        this.logDir = source["logDir"];
	        this.logPath = source["logPath"];
	        this.canShutdown = source["canShutdown"];
	    }
	}
	
	export class SessionSummary {
	    movies: number;
	    episodes: number;
	    completed: number;
	    failed: number;
	    skipped: number;
	    pending: number;
	    needsAttention: boolean;
	    savedAt: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.movies = source["movies"];
	        this.episodes = source["episodes"];
	        this.completed = source["completed"];
	        this.failed = source["failed"];
	        this.skipped = source["skipped"];
	        this.pending = source["pending"];
	        this.needsAttention = source["needsAttention"];
	        this.savedAt = source["savedAt"];
	        this.version = source["version"];
	    }
	}
	export class SessionEpisode {
	    id: string;
	    name: string;
	    number: number;
	    pageUrl: string;
	    streamUrl?: string;
	    streams?: MediaStream[];
	    outputDir?: string;
	    selected: boolean;
	    status?: string;
	    message?: string;
	    output?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionEpisode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.number = source["number"];
	        this.pageUrl = source["pageUrl"];
	        this.streamUrl = source["streamUrl"];
	        this.streams = this.convertValues(source["streams"], MediaStream);
	        this.outputDir = source["outputDir"];
	        this.selected = source["selected"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.output = source["output"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionMovie {
	    key: string;
	    title: string;
	    source: string;
	    pageUrl: string;
	    outputDir: string;
	    collapsed: boolean;
	    episodes: SessionEpisode[];
	
	    static createFrom(source: any = {}) {
	        return new SessionMovie(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.title = source["title"];
	        this.source = source["source"];
	        this.pageUrl = source["pageUrl"];
	        this.outputDir = source["outputDir"];
	        this.collapsed = source["collapsed"];
	        this.episodes = this.convertValues(source["episodes"], SessionEpisode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionState {
	    version: string;
	    savedAt: string;
	    finished: boolean;
	    movies: SessionMovie[];
	
	    static createFrom(source: any = {}) {
	        return new SessionState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.savedAt = source["savedAt"];
	        this.finished = source["finished"];
	        this.movies = this.convertValues(source["movies"], SessionMovie);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SavedSession {
	    state: SessionState;
	    summary: SessionSummary;
	
	    static createFrom(source: any = {}) {
	        return new SavedSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = this.convertValues(source["state"], SessionState);
	        this.summary = this.convertValues(source["summary"], SessionSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class ShutdownStatus {
	    scheduled: boolean;
	    seconds: number;
	    survivesAppExit: boolean;
	    at: string;
	
	    static createFrom(source: any = {}) {
	        return new ShutdownStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scheduled = source["scheduled"];
	        this.seconds = source["seconds"];
	        this.survivesAppExit = source["survivesAppExit"];
	        this.at = source["at"];
	    }
	}
	export class SourceDocument {
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}

}

