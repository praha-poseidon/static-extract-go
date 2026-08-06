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
}

type Let struct {
	Name     string
	Sources  []Source
	Fallback string
}

type Source struct {
	From []string
	Take []string
}

type BuildExpr struct {
	Ref   string
	Const string
}

type TraceEntry struct {
	From  []string
	When  [][]string
	Lets  []Let
	Build map[string]BuildExpr
}
