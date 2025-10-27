export namespace exchange {
	
	export class Position {
	    Symbol: string;
	    Side: string;
	    Size: number;
	    EntryPrice: number;
	    UnrealisedPnl: number;
	    CumRealisedPnl: number;
	
	    static createFrom(source: any = {}) {
	        return new Position(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Symbol = source["Symbol"];
	        this.Side = source["Side"];
	        this.Size = source["Size"];
	        this.EntryPrice = source["EntryPrice"];
	        this.UnrealisedPnl = source["UnrealisedPnl"];
	        this.CumRealisedPnl = source["CumRealisedPnl"];
	    }
	}

}

