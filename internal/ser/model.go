package ser

type Rule struct {
	Name              string
	FactType          string
	EndpointType      string
	EndpointDirection string
	Find              []string
	Where             [][]string
	When              [][]string
	Lets              []Let
	Build             map[string]BuildExpr
	Trace             []TraceEntry
	// IdentityDict from SER: dict { importPath.Func() = value }
	IdentityDict map[string]string
}

type Let struct {
	Name     string
	Sources  []Source
	Fallback string
	// Map is SER map { k: v } after sources. Empty map = no mapping.
	// Apply: hit → value; non-empty map miss → "" (no passthrough; aligned with Java).
	Map map[string]string
}

type Source struct {
	From []string
	Take []string
}

type BuildExpr struct {
	Ref   string // let 名引用
	Const string // 字符串常量
	// Raw 为完整右侧表达式，如 concat(base, path) | normalize slash
	// 非空时优先于 Ref/Const 由引擎求值
	Raw string
}

type TraceEntry struct {
	From  []string
	When  [][]string
	Lets  []Let
	Build map[string]BuildExpr
}
