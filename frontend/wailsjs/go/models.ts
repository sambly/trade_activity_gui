export namespace exchange {
	
	export class Position {
	    Symbol: string;
	    CreatedTime: string;
	    Side: string;
	    Size: number;
	    EntryPrice: number;
	    UnrealisedPnl: number;
	    CumRealisedPnl: number;
	    CurrentPrice: number;
	    CurrentValue: number;
	
	    static createFrom(source: any = {}) {
	        return new Position(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Symbol = source["Symbol"];
	        this.CreatedTime = source["CreatedTime"];
	        this.Side = source["Side"];
	        this.Size = source["Size"];
	        this.EntryPrice = source["EntryPrice"];
	        this.UnrealisedPnl = source["UnrealisedPnl"];
	        this.CumRealisedPnl = source["CumRealisedPnl"];
	        this.CurrentPrice = source["CurrentPrice"];
	        this.CurrentValue = source["CurrentValue"];
	    }
	}

}

