export namespace core {
	
	export class AppSettings {
	    embedding_strategy: string;
	    max_chunks_per_file: number;
	    hotkey: string;
	    ignored_paths: string[];
	    allowed_extensions: string[];
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.embedding_strategy = source["embedding_strategy"];
	        this.max_chunks_per_file = source["max_chunks_per_file"];
	        this.hotkey = source["hotkey"];
	        this.ignored_paths = source["ignored_paths"];
	        this.allowed_extensions = source["allowed_extensions"];
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

