export namespace core {
	
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

