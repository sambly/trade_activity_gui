export namespace exchange {
	
	export class LimitCloseSummary {
	    Symbol: string;
	    DurationSeconds: number;
	    StartSize: number;
	    FilledQty: number;
	    Progress: number;
	    MinPrice: number;
	    MaxPrice: number;
	    AvgPrice: number;
	    FillCount: number;
	
	    static createFrom(source: any = {}) {
	        return new LimitCloseSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Symbol = source["Symbol"];
	        this.DurationSeconds = source["DurationSeconds"];
	        this.StartSize = source["StartSize"];
	        this.FilledQty = source["FilledQty"];
	        this.Progress = source["Progress"];
	        this.MinPrice = source["MinPrice"];
	        this.MaxPrice = source["MaxPrice"];
	        this.AvgPrice = source["AvgPrice"];
	        this.FillCount = source["FillCount"];
	    }
	}
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
	    PriceChangePercent: number;
	    LimitCloseActive: boolean;
	    LimitCloseProgress: number;
	    LimitCloseError: string;
	
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
	        this.PriceChangePercent = source["PriceChangePercent"];
	        this.LimitCloseActive = source["LimitCloseActive"];
	        this.LimitCloseProgress = source["LimitCloseProgress"];
	        this.LimitCloseError = source["LimitCloseError"];
	    }
	}

}

