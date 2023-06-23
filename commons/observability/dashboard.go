package observability

type Annotation struct {
	List []List `json:"list"`
}

type List struct {
	BuiltIn    int    `json:"builtIn"`
	Datasource string `json:"datasource"`
	Enable     bool   `json:"enable"`
	Hide       bool   `json:"hide"`
	IconColor  string `json:"iconColor"`
	Limit      int    `json:"limit"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

type Legend struct {
	AlignAsTable bool     `json:"alignAsTable,omitempty"`
	Avg          bool     `json:"avg,omitempty"`
	Current      bool     `json:"current,omitempty"`
	Max          bool     `json:"max,omitempty"`
	Min          bool     `json:"min,omitempty"`
	Show         bool     `json:"show,omitempty"`
	Total        bool     `json:"total,omitempty"`
	Values       bool     `json:"values,omitempty"`
	Calcs        []string `json:"calcs,omitempty"`
	DisplayMode  string   `json:"displayMode,omitempty"`
	Placement    string   `json:"placement,omitempty"`
}

type ReduceOptions struct {
	Calcs  []string `json:"calcs"`
	Fields string   `json:"fields"`
	Values bool     `json:"values"`
}

type Options struct {
	Num0 struct {
		Text string `json:"text,omitempty"`
	} `json:"0,omitempty"`
	Num1 struct {
		Text string `json:"text"`
	} `json:"1,omitempty"`
	ColorMode      string        `json:"colorMode,omitempty"`
	GraphMode      string        `json:"graphMode,omitempty"`
	JustifyMode    string        `json:"justifyMode,omitempty"`
	Orientation    string        `json:"orientation,omitempty"`
	ReduceOptions  ReduceOptions `json:"reduceOptions,omitempty"`
	Text           struct{}      `json:"text"`
	TextMode       string        `json:"textMode,omitempty"`
	AlertThreshold bool          `json:"alertThreshold,omitempty"`
	Legend         Legend        `json:"legend,omitempty"`
	Tooltip        Tooltip       `json:"tooltip,omitempty"`
}

type GridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

type Color struct {
	Mode string `json:"mode"`
}

type Mappings struct {
	Options Options `json:"options"`
	Type    string  `json:"type"`
}

type Thresholds struct {
	Mode  string  `json:"mode"`
	Steps []Steps `json:"steps"`
}

type Steps struct {
	Color string      `json:"color"`
	Value interface{} `json:"value,omitempty"`
}
type Defaults struct {
	Color      Color      `json:"color"`
	Mappings   []Mappings `json:"mappings"`
	Custom     Custom     `json:"custom"`
	Thresholds Thresholds `json:"thresholds"`
	Unit       string     `json:"unit"`
}

type HideFrom struct {
	Legend  bool `json:"legend"`
	Tooltip bool `json:"tooltip"`
	Viz     bool `json:"viz"`
}

type ScaleDistribution struct {
	Type string `json:"type"`
}

type Stacking struct {
	Group string `json:"group"`
	Mode  string `json:"mode"`
}

type ThresholdStyle struct {
	Mode string `json:"mode"`
}

type Custom struct {
	AxisLabel         string            `json:"axisLabel"`
	AxisPlacement     string            `json:"axisPlacement"`
	BarAlignment      int               `json:"barAlignment"`
	DrawStyle         string            `json:"drawStyle"`
	FillOpacity       int               `json:"fillOpacity"`
	GradientMode      string            `json:"gradientMode"`
	HideFrom          HideFrom          `json:"hideFrom"`
	LineInterpolation string            `json:"lineInterpolation"`
	LineWidth         int               `json:"lineWidth"`
	PointSize         int               `json:"pointSize"`
	ScaleDistribution ScaleDistribution `json:"scaleDistribution"`
	ShowPoints        string            `json:"showPoints"`
	SpanNulls         bool              `json:"spanNulls"`
	Stacking          Stacking          `json:"stacking"`
	ThresholdsStyle   ThresholdStyle    `json:"thresholdsStyle"`
}

type FieldConfig struct {
	Defaults  Defaults      `json:"defaults"`
	Overrides []interface{} `json:"overrides"`
}

type Targets struct {
	Exemplar       bool   `json:"exemplar"`
	Expr           string `json:"expr"`
	Interval       string `json:"interval"`
	IntervalFactor int    `json:"intervalFactor,omitempty"`
	LegendFormat   string `json:"legendFormat"`
	RefID          string `json:"refId"`
}

type Tooltip struct {
	Shared    bool   `json:"shared,omitempty"`
	Sort      int    `json:"sort,omitempty"`
	ValueType string `json:"value_type,omitempty"`
	Mode      string `json:"mode,omitempty"`
}

type XAxis struct {
	Buckets interface{}   `json:"buckets"`
	Mode    string        `json:"mode"`
	Name    interface{}   `json:"name"`
	Show    bool          `json:"show"`
	Values  []interface{} `json:"values"`
}

type YAxis struct {
	Align      bool        `json:"align"`
	AlignLevel interface{} `json:"alignLevel"`
}

type YAxes struct {
	Format  string      `json:"format"`
	Label   interface{} `json:"label"`
	LogBase int         `json:"logBase"`
	Max     interface{} `json:"max"`
	Min     interface{} `json:"min"`
	Show    bool        `json:"show"`
}

type Panels struct {
	Collapsed     bool          `json:"collapsed,omitempty"`
	Datasource    interface{}   `json:"datasource"`
	GridPos       GridPos       `json:"gridPos"`
	ID            int           `json:"id"`
	Panels        []interface{} `json:"panels,omitempty"`
	Title         string        `json:"title"`
	Type          string        `json:"type"`
	CacheTimeout  interface{}   `json:"cacheTimeout,omitempty"`
	FieldConfig   FieldConfig   `json:"fieldConfig,omitempty"`
	Interval      interface{}   `json:"interval,omitempty"`
	Links         []interface{} `json:"links,omitempty"`
	MaxDataPoints int           `json:"maxDataPoints,omitempty"`
	Options       Options       `json:"options,omitempty"`
	PluginVersion string        `json:"pluginVersion,omitempty"`
	Targets       []Targets     `json:"targets,omitempty"`
	TimeFrom      interface{}   `json:"timeFrom,omitempty"`
	TimeShift     interface{}   `json:"timeShift,omitempty"`
	Description   string        `json:"description,omitempty"`
	AliasColors   struct {
	} `json:"aliasColors,omitempty"`
	Bars             bool          `json:"bars,omitempty"`
	DashLength       int           `json:"dashLength,omitempty"`
	Dashes           bool          `json:"dashes,omitempty"`
	Fill             int           `json:"fill,omitempty"`
	FillGradient     int           `json:"fillGradient,omitempty"`
	HiddenSeries     bool          `json:"hiddenSeries,omitempty"`
	Legend           Legend        `json:"legend,omitempty"`
	Lines            bool          `json:"lines,omitempty"`
	Linewidth        int           `json:"linewidth,omitempty"`
	NullPointMode    string        `json:"nullPointMode,omitempty"`
	Percentage       bool          `json:"percentage,omitempty"`
	Pointradius      int           `json:"pointradius,omitempty"`
	Points           bool          `json:"points,omitempty"`
	Renderer         string        `json:"renderer,omitempty"`
	SeriesOverrides  []interface{} `json:"seriesOverrides,omitempty"`
	SpaceLength      int           `json:"spaceLength,omitempty"`
	Stack            bool          `json:"stack,omitempty"`
	SteppedLine      bool          `json:"steppedLine,omitempty"`
	Thresholds       []interface{} `json:"thresholds,omitempty"`
	TimeRegions      []interface{} `json:"timeRegions,omitempty"`
	Tooltip          Tooltip       `json:"tooltip,omitempty"`
	Xaxis            XAxis         `json:"xaxis,omitempty"`
	Yaxes            []YAxes       `json:"yaxes,omitempty"`
	Yaxis            YAxis         `json:"yaxis,omitempty"`
	HideTimeOverride bool          `json:"hideTimeOverride,omitempty"`
	Alert            struct {
		AlertRuleTags struct {
		} `json:"alertRuleTags"`
		Conditions []struct {
			Evaluator struct {
				Params []float64 `json:"params"`
				Type   string    `json:"type"`
			} `json:"evaluator"`
			Operator struct {
				Type string `json:"type"`
			} `json:"operator"`
			Query struct {
				Params []string `json:"params"`
			} `json:"query"`
			Reducer struct {
				Params []interface{} `json:"params"`
				Type   string        `json:"type"`
			} `json:"reducer"`
			Type string `json:"type"`
		} `json:"conditions"`
		ExecutionErrorState string        `json:"executionErrorState"`
		For                 string        `json:"for"`
		Frequency           string        `json:"frequency"`
		Handler             int           `json:"handler"`
		Message             string        `json:"message"`
		Name                string        `json:"name"`
		NoDataState         string        `json:"noDataState"`
		Notifications       []interface{} `json:"notifications"`
	} `json:"alert,omitempty"`
	Transformations []interface{} `json:"transformations,omitempty"`
}

type Dashboard struct {
	Annotations   Annotation    `json:"annotations"`
	Description   string        `json:"description"`
	Editable      bool          `json:"editable"`
	GnetID        int           `json:"gnetId"`
	GraphTooltip  int           `json:"graphTooltip"`
	ID            int           `json:"id"`
	Links         []interface{} `json:"links"`
	Panels        []Panels      `json:"panels"`
	Refresh       bool          `json:"refresh"`
	SchemaVersion int           `json:"schemaVersion"`
	Style         string        `json:"style"`
	Tags          []interface{} `json:"tags"`
	Templating    struct {
		List []interface{} `json:"list"`
	} `json:"templating"`
	Time struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"time"`
	Timepicker struct {
		RefreshIntervals []string `json:"refresh_intervals"`
		TimeOptions      []string `json:"time_options"`
	} `json:"timepicker"`
	Timezone string `json:"timezone"`
	Title    string `json:"title"`
	UID      string `json:"uid"`
	Version  int    `json:"version"`
}
