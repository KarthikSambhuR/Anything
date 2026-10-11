export namespace core {
	
	export class AppSettings {
	    embedding_strategy: string;
	    embedding_model: string;
	    ai_enabled: boolean;
	    gpu_device_id: number;
	    max_chunks_per_file: number;
	    hotkey: string;
	    ignored_paths: string[];
	    allowed_extensions: string[];
	    indexed_directories: string[];
	    priority_directories: string[];
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.embedding_strategy = source["embedding_strategy"];
	        this.embedding_model = source["embedding_model"];
	        this.ai_enabled = source["ai_enabled"];
	        this.gpu_device_id = source["gpu_device_id"];
	        this.max_chunks_per_file = source["max_chunks_per_file"];
	        this.hotkey = source["hotkey"];
	        this.ignored_paths = source["ignored_paths"];
	        this.allowed_extensions = source["allowed_extensions"];
	        this.indexed_directories = source["indexed_directories"];
	        this.priority_directories = source["priority_directories"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    name: string;
	    description: string;
	    downloaded: boolean;
	    size_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.downloaded = source["downloaded"];
	        this.size_bytes = source["size_bytes"];
	    }
	}
	export class SearchResult {
	    Path: string;
	    Snippet: string;
	    Score: number;
	    IconData: string;
	    Extension: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Snippet = source["Snippet"];
	        this.Score = source["Score"];
	        this.IconData = source["IconData"];
	        this.Extension = source["Extension"];
	    }
	}

}

