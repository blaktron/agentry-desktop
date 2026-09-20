export namespace build {
	
	export class IntakeRow {
	    id: string;
	    prompt: string;
	    type: string;
	    required: boolean;
	    choices: string[];
	
	    static createFrom(source: any = {}) {
	        return new IntakeRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.choices = source["choices"];
	    }
	}
	export class MCPServerRow {
	    source: string;
	    package: string;
	    version: string;
	    tools: string[];
	    secrets: string[];
	    egress: string[];
	
	    static createFrom(source: any = {}) {
	        return new MCPServerRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.package = source["package"];
	        this.version = source["version"];
	        this.tools = source["tools"];
	        this.secrets = source["secrets"];
	        this.egress = source["egress"];
	    }
	}
	export class Model {
	    name: string;
	    namespace: string;
	    modes: string[];
	    version: string;
	    summary: string;
	    license: string;
	    homepage?: string;
	    tags: string[];
	    instructionFile: string;
	    instructions: string;
	    intake: IntakeRow[];
	    opening: string;
	    egressMode: string;
	    egressHosts: string[];
	    fsRead: string;
	    fsReadPaths: string[];
	    fsWrite: string;
	    fsWritePaths: string[];
	    execCommands: string[];
	    secrets: string[];
	    dataAccess: string[];
	    mcpServers: MCPServerRow[];
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.modes = source["modes"];
	        this.version = source["version"];
	        this.summary = source["summary"];
	        this.license = source["license"];
	        this.homepage = source["homepage"];
	        this.tags = source["tags"];
	        this.instructionFile = source["instructionFile"];
	        this.instructions = source["instructions"];
	        this.intake = this.convertValues(source["intake"], IntakeRow);
	        this.opening = source["opening"];
	        this.egressMode = source["egressMode"];
	        this.egressHosts = source["egressHosts"];
	        this.fsRead = source["fsRead"];
	        this.fsReadPaths = source["fsReadPaths"];
	        this.fsWrite = source["fsWrite"];
	        this.fsWritePaths = source["fsWritePaths"];
	        this.execCommands = source["execCommands"];
	        this.secrets = source["secrets"];
	        this.dataAccess = source["dataAccess"];
	        this.mcpServers = this.convertValues(source["mcpServers"], MCPServerRow);
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
	export class PublishResult {
	    version: string;
	    status: string;
	    digest: string;
	    detail?: string;
	    refusal?: string;
	    findingCounts?: {[key: string]: number};
	
	    static createFrom(source: any = {}) {
	        return new PublishResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.status = source["status"];
	        this.digest = source["digest"];
	        this.detail = source["detail"];
	        this.refusal = source["refusal"];
	        this.findingCounts = source["findingCounts"];
	    }
	}

}

export namespace install {
	
	export class Item {
	    registry: string;
	    namespace: string;
	    name: string;
	    target: string;
	    dir: string;
	    version: string;
	    // Go type: time
	    installedAt: any;
	    capabilities: installed.CapSnapshot;
	    managed?: boolean;
	    present: boolean;
	    manageable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.dir = source["dir"];
	        this.version = source["version"];
	        this.installedAt = this.convertValues(source["installedAt"], null);
	        this.capabilities = this.convertValues(source["capabilities"], installed.CapSnapshot);
	        this.managed = source["managed"];
	        this.present = source["present"];
	        this.manageable = source["manageable"];
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
	export class Paths {
	    dest: string;
	    managed: boolean;
	    agentsMd?: string;
	
	    static createFrom(source: any = {}) {
	        return new Paths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dest = source["dest"];
	        this.managed = source["managed"];
	        this.agentsMd = source["agentsMd"];
	    }
	}
	export class Plan {
	    id: string;
	    registry: string;
	    namespace: string;
	    name: string;
	    version: string;
	    modes: string[];
	    summary: string;
	    digest: string;
	    yanked: boolean;
	    attestation: trust.AttestationResult;
	    unsigned: boolean;
	    capabilities: installed.CapSnapshot;
	    needsCapabilityConfirm: boolean;
	    prevVersion?: string;
	    diff?: installed.Diff;
	    requiresWidenedConfirm: boolean;
	    vulnerabilities: mcpvuln.Status[];
	    vulnerabilityLine?: string;
	    vulnerabilityRefusal?: string;
	    target: string;
	    paths: Paths;
	    targetWarning?: string;
	    secrets?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.modes = source["modes"];
	        this.summary = source["summary"];
	        this.digest = source["digest"];
	        this.yanked = source["yanked"];
	        this.attestation = this.convertValues(source["attestation"], trust.AttestationResult);
	        this.unsigned = source["unsigned"];
	        this.capabilities = this.convertValues(source["capabilities"], installed.CapSnapshot);
	        this.needsCapabilityConfirm = source["needsCapabilityConfirm"];
	        this.prevVersion = source["prevVersion"];
	        this.diff = this.convertValues(source["diff"], installed.Diff);
	        this.requiresWidenedConfirm = source["requiresWidenedConfirm"];
	        this.vulnerabilities = this.convertValues(source["vulnerabilities"], mcpvuln.Status);
	        this.vulnerabilityLine = source["vulnerabilityLine"];
	        this.vulnerabilityRefusal = source["vulnerabilityRefusal"];
	        this.target = source["target"];
	        this.paths = this.convertValues(source["paths"], Paths);
	        this.targetWarning = source["targetWarning"];
	        this.secrets = source["secrets"];
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
	export class Result {
	    message: string;
	    dest: string;
	    secretsNote?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.dest = source["dest"];
	        this.secretsNote = source["secretsNote"];
	    }
	}
	export class UpdateInfo {
	    key: installed.Key;
	    current: string;
	    latest?: string;
	    updateAvailable: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = this.convertValues(source["key"], installed.Key);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.updateAvailable = source["updateAvailable"];
	        this.error = source["error"];
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

}

export namespace installed {
	
	export class CapSnapshot {
	    egressHosts?: string[];
	    egressNone?: boolean;
	    fsRead?: string;
	    fsReadPaths?: string[];
	    fsWrite?: string;
	    fsWritePaths?: string[];
	    execCommands?: string[];
	    secrets?: string[];
	    dataAccess?: string[];
	    mcpServers?: string[];
	    mcpServerEnv?: {[key: string]: any};
	
	    static createFrom(source: any = {}) {
	        return new CapSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.egressHosts = source["egressHosts"];
	        this.egressNone = source["egressNone"];
	        this.fsRead = source["fsRead"];
	        this.fsReadPaths = source["fsReadPaths"];
	        this.fsWrite = source["fsWrite"];
	        this.fsWritePaths = source["fsWritePaths"];
	        this.execCommands = source["execCommands"];
	        this.secrets = source["secrets"];
	        this.dataAccess = source["dataAccess"];
	        this.mcpServers = source["mcpServers"];
	        this.mcpServerEnv = source["mcpServerEnv"];
	    }
	}
	export class Diff {
	    widened?: string[];
	    narrowed?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Diff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.widened = source["widened"];
	        this.narrowed = source["narrowed"];
	    }
	}
	export class Key {
	    registry: string;
	    namespace: string;
	    name: string;
	    target: string;
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new Key(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.dir = source["dir"];
	    }
	}
	export class Record {
	    registry: string;
	    namespace: string;
	    name: string;
	    target: string;
	    dir: string;
	    version: string;
	    // Go type: time
	    installedAt: any;
	    capabilities: CapSnapshot;
	    managed?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Record(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.dir = source["dir"];
	        this.version = source["version"];
	        this.installedAt = this.convertValues(source["installedAt"], null);
	        this.capabilities = this.convertValues(source["capabilities"], CapSnapshot);
	        this.managed = source["managed"];
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

}

export namespace main {
	
	export class SubscriptionView {
	    tier: string;
	    balance: number;
	    allowance: number;
	    submissionCost: number;
	    periodEnd: string;
	    pendingTier?: string;
	    // Go type: struct { Max int64 "json:\"max\""; Used int64 "json:\"used\"" }
	    privateAgents: any;
	    upgradeUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tier = source["tier"];
	        this.balance = source["balance"];
	        this.allowance = source["allowance"];
	        this.submissionCost = source["submissionCost"];
	        this.periodEnd = source["periodEnd"];
	        this.pendingTier = source["pendingTier"];
	        this.privateAgents = this.convertValues(source["privateAgents"], Object);
	        this.upgradeUrl = source["upgradeUrl"];
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
	export class UserView {
	    handle: string;
	    displayName: string;
	    email: string;
	
	    static createFrom(source: any = {}) {
	        return new UserView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.handle = source["handle"];
	        this.displayName = source["displayName"];
	        this.email = source["email"];
	    }
	}
	export class AccountView {
	    user: UserView;
	    subscription?: SubscriptionView;
	    subscriptionError?: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], UserView);
	        this.subscription = this.convertValues(source["subscription"], SubscriptionView);
	        this.subscriptionError = source["subscriptionError"];
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
	export class DependencyAlertView {
	    id: string;
	    version: string;
	    ecosystem: string;
	    package: string;
	    packageVersion: string;
	    severity: string;
	    advisoryIds: string[];
	    fixedIn: string[];
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DependencyAlertView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.ecosystem = source["ecosystem"];
	        this.package = source["package"];
	        this.packageVersion = source["packageVersion"];
	        this.severity = source["severity"];
	        this.advisoryIds = source["advisoryIds"];
	        this.fixedIn = source["fixedIn"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class VersionView {
	    version: string;
	    channel: string;
	    status: string;
	    createdAt: string;
	    publishedAt?: string;
	    archiveDigest?: string;
	    yanked: boolean;
	    yankReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.channel = source["channel"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.publishedAt = source["publishedAt"];
	        this.archiveDigest = source["archiveDigest"];
	        this.yanked = source["yanked"];
	        this.yankReason = source["yankReason"];
	    }
	}
	export class AgentView {
	    namespace: string;
	    name: string;
	    modes: string[];
	    summary: string;
	    visibility: string;
	    quarantine?: string;
	    createdAt: string;
	    gitHttpUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.modes = source["modes"];
	        this.summary = source["summary"];
	        this.visibility = source["visibility"];
	        this.quarantine = source["quarantine"];
	        this.createdAt = source["createdAt"];
	        this.gitHttpUrl = source["gitHttpUrl"];
	    }
	}
	export class AgentDetailView {
	    agent: AgentView;
	    versions: VersionView[];
	    latest: string;
	    latestCapabilities?: {[key: string]: any};
	    latestDigest?: string;
	    dependencyAlerts: DependencyAlertView[];
	
	    static createFrom(source: any = {}) {
	        return new AgentDetailView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent = this.convertValues(source["agent"], AgentView);
	        this.versions = this.convertValues(source["versions"], VersionView);
	        this.latest = source["latest"];
	        this.latestCapabilities = source["latestCapabilities"];
	        this.latestDigest = source["latestDigest"];
	        this.dependencyAlerts = this.convertValues(source["dependencyAlerts"], DependencyAlertView);
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
	
	
	export class FindingView {
	    scanner: string;
	    severity: string;
	    confidence: string;
	    title: string;
	    filePath?: string;
	    lineStart?: number;
	    lineEnd?: number;
	    excerpt?: string;
	    verdict: string;
	    verdictReason?: string;
	
	    static createFrom(source: any = {}) {
	        return new FindingView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.confidence = source["confidence"];
	        this.title = source["title"];
	        this.filePath = source["filePath"];
	        this.lineStart = source["lineStart"];
	        this.lineEnd = source["lineEnd"];
	        this.excerpt = source["excerpt"];
	        this.verdict = source["verdict"];
	        this.verdictReason = source["verdictReason"];
	    }
	}
	export class HarnessView {
	    name: string;
	    displayName: string;
	    executable: string;
	    installUrl: string;
	    found: boolean;
	    path?: string;
	    pathFromPolicy: boolean;
	    version?: string;
	    versionMatches: boolean;
	    warnings?: string[];
	    reason?: string;
	    chosen: boolean;
	    mcpCommand: string;
	
	    static createFrom(source: any = {}) {
	        return new HarnessView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.executable = source["executable"];
	        this.installUrl = source["installUrl"];
	        this.found = source["found"];
	        this.path = source["path"];
	        this.pathFromPolicy = source["pathFromPolicy"];
	        this.version = source["version"];
	        this.versionMatches = source["versionMatches"];
	        this.warnings = source["warnings"];
	        this.reason = source["reason"];
	        this.chosen = source["chosen"];
	        this.mcpCommand = source["mcpCommand"];
	    }
	}
	export class HostedArtefactView {
	    name: string;
	    bytes: number;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedArtefactView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.bytes = source["bytes"];
	        this.path = source["path"];
	    }
	}
	export class HostedCapsView {
	    timeoutSeconds: number;
	    maxTurns: number;
	    maxTokens: number;
	
	    static createFrom(source: any = {}) {
	        return new HostedCapsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.maxTokens = source["maxTokens"];
	    }
	}
	export class HostedAvailabilityView {
	    offered: boolean;
	    available: boolean;
	    reason?: string;
	    remedy?: string;
	    signedIn: boolean;
	    extension: boolean;
	    enabled: boolean;
	    workersOnline: number;
	    queued: number;
	    tier?: string;
	    holdCredits: number;
	    tokensPerCredit: number;
	    caps?: HostedCapsView;
	    balance: number;
	    balanceKnown: boolean;
	    upgradeUrl?: string;
	    maxInputFiles: number;
	    maxInputBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new HostedAvailabilityView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offered = source["offered"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.remedy = source["remedy"];
	        this.signedIn = source["signedIn"];
	        this.extension = source["extension"];
	        this.enabled = source["enabled"];
	        this.workersOnline = source["workersOnline"];
	        this.queued = source["queued"];
	        this.tier = source["tier"];
	        this.holdCredits = source["holdCredits"];
	        this.tokensPerCredit = source["tokensPerCredit"];
	        this.caps = this.convertValues(source["caps"], HostedCapsView);
	        this.balance = source["balance"];
	        this.balanceKnown = source["balanceKnown"];
	        this.upgradeUrl = source["upgradeUrl"];
	        this.maxInputFiles = source["maxInputFiles"];
	        this.maxInputBytes = source["maxInputBytes"];
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
	
	export class HostedRunView {
	    id: string;
	    ref: string;
	    digest: string;
	    status: string;
	    statusLine: string;
	    queuePosition?: number;
	    cancelRequested: boolean;
	    outcome?: string;
	    exitClass?: string;
	    outcomeLine?: string;
	    outcomeTone?: string;
	    error?: string;
	    turns: number;
	    inputTokens: number;
	    outputTokens: number;
	    totalTokens: number;
	    costUsd: number;
	    wallMs: number;
	    holdCredits: number;
	    settledCredits: number;
	    settled: boolean;
	    capabilitiesEnforced?: {[key: string]: string};
	    attestationState?: string;
	    adapter?: string;
	    resultJson?: string;
	    outDir?: string;
	    outputs: HostedArtefactView[];
	    downloadError?: string;
	    caps?: HostedCapsView;
	    createdAt?: string;
	    finishedAt?: string;
	    final: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostedRunView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.status = source["status"];
	        this.statusLine = source["statusLine"];
	        this.queuePosition = source["queuePosition"];
	        this.cancelRequested = source["cancelRequested"];
	        this.outcome = source["outcome"];
	        this.exitClass = source["exitClass"];
	        this.outcomeLine = source["outcomeLine"];
	        this.outcomeTone = source["outcomeTone"];
	        this.error = source["error"];
	        this.turns = source["turns"];
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.costUsd = source["costUsd"];
	        this.wallMs = source["wallMs"];
	        this.holdCredits = source["holdCredits"];
	        this.settledCredits = source["settledCredits"];
	        this.settled = source["settled"];
	        this.capabilitiesEnforced = source["capabilitiesEnforced"];
	        this.attestationState = source["attestationState"];
	        this.adapter = source["adapter"];
	        this.resultJson = source["resultJson"];
	        this.outDir = source["outDir"];
	        this.outputs = this.convertValues(source["outputs"], HostedArtefactView);
	        this.downloadError = source["downloadError"];
	        this.caps = this.convertValues(source["caps"], HostedCapsView);
	        this.createdAt = source["createdAt"];
	        this.finishedAt = source["finishedAt"];
	        this.final = source["final"];
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
	export class HostedStartView {
	    run?: HostedRunView;
	    refusal?: string;
	    detail?: string;
	    balance?: number;
	    hold?: number;
	    resetsAt?: string;
	    upgradeUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostedStartView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], HostedRunView);
	        this.refusal = source["refusal"];
	        this.detail = source["detail"];
	        this.balance = source["balance"];
	        this.hold = source["hold"];
	        this.resetsAt = source["resetsAt"];
	        this.upgradeUrl = source["upgradeUrl"];
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
	export class IntakeQuestionView {
	    id: string;
	    prompt: string;
	    type: string;
	    required: boolean;
	    choices: string[];
	
	    static createFrom(source: any = {}) {
	        return new IntakeQuestionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.choices = source["choices"];
	    }
	}
	export class MCPToolRow {
	    tool: string;
	    grantedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPToolRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tool = source["tool"];
	        this.grantedBy = source["grantedBy"];
	    }
	}
	export class MCPView {
	    ref: string;
	    version: string;
	    toolRows: MCPToolRow[];
	    servers: string[];
	    secretNames: string[];
	    dataAccess: string[];
	    embeddedTools: string[];
	    capabilityLines: string[];
	    vulnerabilities: mcpvuln.Status[];
	    vulnerabilityLine?: string;
	    refusal?: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.version = source["version"];
	        this.toolRows = this.convertValues(source["toolRows"], MCPToolRow);
	        this.servers = source["servers"];
	        this.secretNames = source["secretNames"];
	        this.dataAccess = source["dataAccess"];
	        this.embeddedTools = source["embeddedTools"];
	        this.capabilityLines = source["capabilityLines"];
	        this.vulnerabilities = this.convertValues(source["vulnerabilities"], mcpvuln.Status);
	        this.vulnerabilityLine = source["vulnerabilityLine"];
	        this.refusal = source["refusal"];
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
	export class MetaView {
	    version: string;
	    apiVersion: string;
	    minCliVersion?: string;
	    features: {[key: string]: boolean};
	
	    static createFrom(source: any = {}) {
	        return new MetaView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.apiVersion = source["apiVersion"];
	        this.minCliVersion = source["minCliVersion"];
	        this.features = source["features"];
	    }
	}
	export class PrereqView {
	    dockerOk: boolean;
	    dockerErr?: string;
	    os: string;
	    distro?: string;
	    canInstall: boolean;
	    command?: string;
	    downloadUrl: string;
	    note?: string;
	    state: string;
	    engineInstalled: boolean;
	    inDockerGroup: boolean;
	    groupCommand?: string;
	
	    static createFrom(source: any = {}) {
	        return new PrereqView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dockerOk = source["dockerOk"];
	        this.dockerErr = source["dockerErr"];
	        this.os = source["os"];
	        this.distro = source["distro"];
	        this.canInstall = source["canInstall"];
	        this.command = source["command"];
	        this.downloadUrl = source["downloadUrl"];
	        this.note = source["note"];
	        this.state = source["state"];
	        this.engineInstalled = source["engineInstalled"];
	        this.inDockerGroup = source["inDockerGroup"];
	        this.groupCommand = source["groupCommand"];
	    }
	}
	export class ProfileStatusView {
	    ref: string;
	    digest: string;
	    declaredSecrets: string[];
	    exists: boolean;
	    keep: boolean;
	    storedSecretNames: string[];
	    capabilityLines: string[];
	    intake: IntakeQuestionView[];
	    modes: string[];
	    opening: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.declaredSecrets = source["declaredSecrets"];
	        this.exists = source["exists"];
	        this.keep = source["keep"];
	        this.storedSecretNames = source["storedSecretNames"];
	        this.capabilityLines = source["capabilityLines"];
	        this.intake = this.convertValues(source["intake"], IntakeQuestionView);
	        this.modes = source["modes"];
	        this.opening = source["opening"];
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
	export class RegistryView {
	    url: string;
	    hasToken: boolean;
	    tokenSource?: string;
	    isDefault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RegistryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.hasToken = source["hasToken"];
	        this.tokenSource = source["tokenSource"];
	        this.isDefault = source["isDefault"];
	    }
	}
	export class ReportView {
	    versionStatus: string;
	    scanStatus?: string;
	    rulesetVersion?: string;
	    judgeModel?: string;
	    finishedAt?: string;
	    findingCounts?: {[key: string]: number};
	    findings: FindingView[];
	
	    static createFrom(source: any = {}) {
	        return new ReportView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versionStatus = source["versionStatus"];
	        this.scanStatus = source["scanStatus"];
	        this.rulesetVersion = source["rulesetVersion"];
	        this.judgeModel = source["judgeModel"];
	        this.finishedAt = source["finishedAt"];
	        this.findingCounts = source["findingCounts"];
	        this.findings = this.convertValues(source["findings"], FindingView);
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
	export class RunResultView {
	    status: string;
	    exitCode: number;
	    error?: string;
	    runId: string;
	    ref: string;
	    digest: string;
	    resultJson: string;
	    outDir: string;
	    receiptsDir: string;
	    runPlanSha: string;
	    inputTokens: number;
	    outputTokens: number;
	    costUsd: number;
	    wallMs: number;
	    turns: number;
	    attestationState: string;
	
	    static createFrom(source: any = {}) {
	        return new RunResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.exitCode = source["exitCode"];
	        this.error = source["error"];
	        this.runId = source["runId"];
	        this.ref = source["ref"];
	        this.digest = source["digest"];
	        this.resultJson = source["resultJson"];
	        this.outDir = source["outDir"];
	        this.receiptsDir = source["receiptsDir"];
	        this.runPlanSha = source["runPlanSha"];
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.costUsd = source["costUsd"];
	        this.wallMs = source["wallMs"];
	        this.turns = source["turns"];
	        this.attestationState = source["attestationState"];
	    }
	}
	export class SearchHitView {
	    namespace: string;
	    name: string;
	    modes: string[];
	    summary: string;
	    installs30d: number;
	    quarantine?: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchHitView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.modes = source["modes"];
	        this.summary = source["summary"];
	        this.installs30d = source["installs30d"];
	        this.quarantine = source["quarantine"];
	    }
	}
	export class SearchQuery {
	    q: string;
	    mode?: string;
	    capability?: string;
	    license?: string;
	    sort?: string;
	    limit?: number;
	    offset?: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.q = source["q"];
	        this.mode = source["mode"];
	        this.capability = source["capability"];
	        this.license = source["license"];
	        this.sort = source["sort"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class SearchView {
	    total?: number;
	    results: SearchHitView[];
	    facets?: {[key: string]: any};
	
	    static createFrom(source: any = {}) {
	        return new SearchView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.results = this.convertValues(source["results"], SearchHitView);
	        this.facets = source["facets"];
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
	export class SelfUpdateView {
	    channel: string;
	    current: string;
	    enabled: boolean;
	    checked?: string;
	    available?: selfupdate.Release;
	    err?: string;
	    downloadUrl?: string;
	    dismissed?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SelfUpdateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel = source["channel"];
	        this.current = source["current"];
	        this.enabled = source["enabled"];
	        this.checked = source["checked"];
	        this.available = this.convertValues(source["available"], selfupdate.Release);
	        this.err = source["err"];
	        this.downloadUrl = source["downloadUrl"];
	        this.dismissed = source["dismissed"];
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
	export class SessionView {
	    ref: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	    }
	}
	export class SettingsView {
	    defaultRegistry: string;
	    registries: RegistryView[];
	    configPath: string;
	    onboarded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SettingsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultRegistry = source["defaultRegistry"];
	        this.registries = this.convertValues(source["registries"], RegistryView);
	        this.configPath = source["configPath"];
	        this.onboarded = source["onboarded"];
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
	export class SetupStatusView {
	    dockerOk: boolean;
	    dockerErr?: string;
	    policyPath: string;
	    policyExists: boolean;
	    provider: string;
	    model: string;
	    apiKeyEnv: string;
	    apiKeySource?: string;
	    keyStored: boolean;
	    keyStoreErr?: string;
	    baseUrl?: string;
	    imagesReady: boolean;
	    timeoutSeconds: number;
	    maxTurns: number;
	    keepProfilesByDefault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SetupStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dockerOk = source["dockerOk"];
	        this.dockerErr = source["dockerErr"];
	        this.policyPath = source["policyPath"];
	        this.policyExists = source["policyExists"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.apiKeyEnv = source["apiKeyEnv"];
	        this.apiKeySource = source["apiKeySource"];
	        this.keyStored = source["keyStored"];
	        this.keyStoreErr = source["keyStoreErr"];
	        this.baseUrl = source["baseUrl"];
	        this.imagesReady = source["imagesReady"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.maxTurns = source["maxTurns"];
	        this.keepProfilesByDefault = source["keepProfilesByDefault"];
	    }
	}
	export class SignInResult {
	    user: UserView;
	    tokenStore: string;
	    pinNote: string;
	
	    static createFrom(source: any = {}) {
	        return new SignInResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], UserView);
	        this.tokenStore = source["tokenStore"];
	        this.pinNote = source["pinNote"];
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
	
	

}

export namespace mcpvuln {
	
	export class Advisory {
	    id: string;
	    severity: string;
	    summary: string;
	    fixedIn: string[];
	
	    static createFrom(source: any = {}) {
	        return new Advisory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.severity = source["severity"];
	        this.summary = source["summary"];
	        this.fixedIn = source["fixedIn"];
	    }
	}
	export class Status {
	    ref: string;
	    status: string;
	    maxSeverity?: string;
	    advisories: Advisory[];
	    checkedAt?: string;
	    error?: string;
	    refuses: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.status = source["status"];
	        this.maxSeverity = source["maxSeverity"];
	        this.advisories = this.convertValues(source["advisories"], Advisory);
	        this.checkedAt = source["checkedAt"];
	        this.error = source["error"];
	        this.refuses = source["refuses"];
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

}

export namespace selfupdate {
	
	export class Release {
	    version: string;
	    commit: string;
	    signedBy: string;
	    retired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.signedBy = source["signedBy"];
	        this.retired = source["retired"];
	    }
	}

}

export namespace trust {
	
	export class AttestationResult {
	    state: string;
	    detail: string;
	    keyId?: string;
	    agent?: string;
	    version?: string;
	    rulesetVersion?: string;
	    reportDigest?: string;
	    judgeModel?: string;
	    source?: string;
	    firstUse?: boolean;
	    keyRotated?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AttestationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.detail = source["detail"];
	        this.keyId = source["keyId"];
	        this.agent = source["agent"];
	        this.version = source["version"];
	        this.rulesetVersion = source["rulesetVersion"];
	        this.reportDigest = source["reportDigest"];
	        this.judgeModel = source["judgeModel"];
	        this.source = source["source"];
	        this.firstUse = source["firstUse"];
	        this.keyRotated = source["keyRotated"];
	    }
	}

}

export namespace validate {
	
	export class InventoryFinding {
	    scanner: string;
	    severity: string;
	    title: string;
	    filePath: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new InventoryFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.filePath = source["filePath"];
	        this.detail = source["detail"];
	    }
	}
	export class Issue {
	    rule: string;
	    path?: string;
	    message: string;
	    warning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.path = source["path"];
	        this.message = source["message"];
	        this.warning = source["warning"];
	    }
	}
	export class ScanFinding {
	    ruleId: string;
	    scanner: string;
	    severity: string;
	    confidence: string;
	    title: string;
	    filePath: string;
	    lineStart: number;
	    lineEnd: number;
	    excerpt: string;
	    taxonomy?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScanFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ruleId = source["ruleId"];
	        this.scanner = source["scanner"];
	        this.severity = source["severity"];
	        this.confidence = source["confidence"];
	        this.title = source["title"];
	        this.filePath = source["filePath"];
	        this.lineStart = source["lineStart"];
	        this.lineEnd = source["lineEnd"];
	        this.excerpt = source["excerpt"];
	        this.taxonomy = source["taxonomy"];
	    }
	}
	export class Result {
	    dir: string;
	    manifestName?: string;
	    manifestModes?: string[];
	    manifestVersion?: string;
	    manifestError?: string;
	    issues: Issue[];
	    inventory: InventoryFinding[];
	    findings: ScanFinding[];
	    bundleVersion: string;
	    bundleSource: string;
	    fileCount: number;
	    readErrors?: string[];
	    errorCount: number;
	    warnCount: number;
	    inventoryFailing: number;
	    passed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.manifestName = source["manifestName"];
	        this.manifestModes = source["manifestModes"];
	        this.manifestVersion = source["manifestVersion"];
	        this.manifestError = source["manifestError"];
	        this.issues = this.convertValues(source["issues"], Issue);
	        this.inventory = this.convertValues(source["inventory"], InventoryFinding);
	        this.findings = this.convertValues(source["findings"], ScanFinding);
	        this.bundleVersion = source["bundleVersion"];
	        this.bundleSource = source["bundleSource"];
	        this.fileCount = source["fileCount"];
	        this.readErrors = source["readErrors"];
	        this.errorCount = source["errorCount"];
	        this.warnCount = source["warnCount"];
	        this.inventoryFailing = source["inventoryFailing"];
	        this.passed = source["passed"];
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

}

