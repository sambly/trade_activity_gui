export namespace exchange {
	
	export class Position {
	    Symbol: string;
	    CreatedTime: string;
	    UpdatedTime: string;
	    PositionIdx: number;
	    Side: string;
	    Size: number;
	    EntryPrice: number;
	    UnrealisedPnl: number;
	    CurrentPrice: number;
	    CurrentValue: number;
	
	    static createFrom(source: any = {}) {
	        return new Position(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Symbol = source["Symbol"];
	        this.CreatedTime = source["CreatedTime"];
	        this.UpdatedTime = source["UpdatedTime"];
	        this.PositionIdx = source["PositionIdx"];
	        this.Side = source["Side"];
	        this.Size = source["Size"];
	        this.EntryPrice = source["EntryPrice"];
	        this.UnrealisedPnl = source["UnrealisedPnl"];
	        this.CurrentPrice = source["CurrentPrice"];
	        this.CurrentValue = source["CurrentValue"];
	    }
	}

}

