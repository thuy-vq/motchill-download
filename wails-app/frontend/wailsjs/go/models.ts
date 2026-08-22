export namespace main {
	
	export class Episode {
	    id: string;
	    name: string;
	    number: number;
	    pageUrl: string;
	    streamUrl?: string;
	    streams?: MediaStream[];
	    engine?: string;
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
	        this.engine = source["engine"];
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
	export class CookieStatus {
	    path: string;
	    count: number;
	    domains: string[];
	
	    static createFrom(source: any = {}) {
	        return new CookieStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.count = source["count"];
	        this.domains = source["domains"];
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
	    engine?: string;
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
	        this.engine = source["engine"];
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
	    maxHeight: number;
	    cookieSource?: string;
	
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
	        this.maxHeight = source["maxHeight"];
	        this.cookieSource = source["cookieSource"];
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
	export class YtDlpTuning {
	    pluginDir: string;
	    providerUrl: string;
	    token: string;
	    playerClient: string;
	
	    static createFrom(source: any = {}) {
	        return new YtDlpTuning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginDir = source["pluginDir"];
	        this.providerUrl = source["providerUrl"];
	        this.token = source["token"];
	        this.playerClient = source["playerClient"];
	    }
	}
	export class ToolStatus {
	    ready: boolean;
	    path: string;
	    version: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.checkedAt = source["checkedAt"];
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
	    ytDlp: ToolStatus;
	    maxHeight: number;
	    cookieSource: string;
	    cookies: CookieStatus;
	    tuning: YtDlpTuning;
	
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
	        this.ytDlp = this.convertValues(source["ytDlp"], ToolStatus);
	        this.maxHeight = source["maxHeight"];
	        this.cookieSource = source["cookieSource"];
	        this.cookies = this.convertValues(source["cookies"], CookieStatus);
	        this.tuning = this.convertValues(source["tuning"], YtDlpTuning);
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
	
	export class PluginCheck {
	    pluginsFound: boolean;
	    tokenUsed: boolean;
	    resolved: boolean;
	    plugins: string[];
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginsFound = source["pluginsFound"];
	        this.tokenUsed = source["tokenUsed"];
	        this.resolved = source["resolved"];
	        this.plugins = source["plugins"];
	        this.message = source["message"];
	    }
	}
	export class ProviderStatus {
	    pluginInstalled: boolean;
	    pluginDir: string;
	    serverInstalled: boolean;
	    serverDir: string;
	    running: boolean;
	    port: number;
	    version: string;
	    nodeVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginInstalled = source["pluginInstalled"];
	        this.pluginDir = source["pluginDir"];
	        this.serverInstalled = source["serverInstalled"];
	        this.serverDir = source["serverDir"];
	        this.running = source["running"];
	        this.port = source["port"];
	        this.version = source["version"];
	        this.nodeVersion = source["nodeVersion"];
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
	    engine?: string;
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
	        this.engine = source["engine"];
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

