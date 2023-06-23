package observability

func generateEventMesh() Panels {
	return Panels{
		Title:      "Event Mesh (Message Propagation)",
		Type:       "row",
		Panels:     nil,
		ID:         84,
		Collapsed:  true,
		Datasource: nil,
		GridPos:    GridPos{H: 1, W: 24, X: 0, Y: 75},
	}
}

func generateCPULoad() Panels {
	return Panels{
		Title:        "CPU Load",
		Datasource:   "Prometheus",
		AliasColors:  struct{}{},
		Bars:         false,
		DashLength:   10,
		Dashes:       false,
		Description:  "",
		Fill:         1,
		FillGradient: 0,
		GridPos:      GridPos{H: 6, W: 7, X: 0, Y: 57},
		HiddenSeries: false,
		ID:           64,
		Legend: Legend{
			AlignAsTable: true,
			Avg:          true,
			Current:      true,
			Max:          true,
			Min:          true,
			Show:         true,
			Total:        false,
			Values:       true,
		},
		Lines:         true,
		Linewidth:     1,
		Links:         nil,
		NullPointMode: "null",
		Options: Options{
			AlertThreshold: true,
		},
		Percentage:      true,
		PluginVersion:   "8.1.5",
		Pointradius:     5,
		Points:          false,
		Renderer:        "flot",
		SeriesOverrides: nil,
		SpaceLength:     10,
		Stack:           false,
		SteppedLine:     false,

		Targets: []Targets{{
			Exemplar:       true,
			Expr:           "base_cpu_systemLoadAverage",
			Interval:       "",
			IntervalFactor: 2,
			LegendFormat:   "CPU Load",
			RefID:          "A",
		}},
		Thresholds:  nil,
		TimeFrom:    nil,
		TimeRegions: nil,
		TimeShift:   nil,
		Tooltip: Tooltip{
			Shared:    true,
			Sort:      0,
			ValueType: "individual",
		},
		Xaxis: XAxis{
			Buckets: nil,
			Mode:    "time",
			Name:    nil,
			Show:    true,
			Values:  nil,
		},
		Yaxes: []YAxes{{
			Format:  "percentunit",
			Label:   nil,
			LogBase: 1,
			Max:     nil,
			Min:     nil,
			Show:    true,
		}, {
			Format:  "short",
			Label:   nil,
			LogBase: 1,
			Max:     nil,
			Min:     nil,
			Show:    false,
		}},
		Yaxis: YAxis{
			Align:      false,
			AlignLevel: nil,
		},
	}
}

func generateDBStatus(dbName string) Panels {
	return Panels{
		CacheTimeout: nil,
		Datasource:   "Prometheus",
		FieldConfig: FieldConfig{
			Defaults: Defaults{
				Color: Color{
					Mode: "thresholds",
				},
				Mappings: []Mappings{
					{
						Options: Options{
							Num0: struct {
								Text string `json:"text,omitempty"`
							}{Text: "DOWN"},
							Num1: struct {
								Text string `json:"text"`
							}{Text: "UP"},
						},
						Type: "value",
					},
				},
				Thresholds: Thresholds{
					Mode: "absolute",
					Steps: []Steps{
						{
							Color: "#d44a3a",
							Value: nil,
						},
						{
							Color: "#37872D",
							Value: 1,
						},
						{
							Color: "#FADE2A",
						},
					}},
				Unit: "none",
			},
			Overrides: nil,
		},
		GridPos:       GridPos{H: 2, W: 4, X: 12, Y: 49},
		ID:            59,
		Interval:      nil,
		Links:         nil,
		MaxDataPoints: 100,
		Options: Options{
			ColorMode:   "background",
			GraphMode:   "none",
			JustifyMode: "auto",
			Orientation: "horizontal",
			ReduceOptions: ReduceOptions{
				Calcs:  []string{"lastNotNull"},
				Fields: "",
				Values: false,
			},
			Text:     struct{}{},
			TextMode: "auto",
		},
		PluginVersion: "8.1.5",
		Targets: []Targets{
			{
				Exemplar:     true,
				Expr:         "oracledb_up{job=\"db-metrics-exporter-" + dbName + "\"}",
				Interval:     "",
				LegendFormat: "",
				RefID:        "A",
			},
		},
		TimeFrom:        nil,
		TimeShift:       nil,
		Title:           "DB Status",
		Transformations: nil,
		Type:            "stat",
	}
}

func generateDBSessions(dbName string) Panels {
	return Panels{
		Title: "DB Sessions",
		Type:  "timeseries",
		Targets: []Targets{{
			Exemplar:     true,
			Expr:         "oracledb_" + dbName + "_sessions_value",
			Interval:     "",
			LegendFormat: "",
			RefID:        "A",
		}},
		ID: 35,
		Options: Options{
			Legend: Legend{
				Calcs:       nil,
				DisplayMode: "list",
				Placement:   "bottom",
			},
			Tooltip: Tooltip{Mode: "single"},
		},
		GridPos:    GridPos{H: 8, W: 12, X: 12, Y: 27},
		Datasource: nil,
		FieldConfig: FieldConfig{
			Defaults: Defaults{
				Color: Color{Mode: "palette-classic"},
				Custom: Custom{
					AxisLabel:     "",
					AxisPlacement: "auto",
					BarAlignment:  0,
					DrawStyle:     "line",
					FillOpacity:   0,
					GradientMode:  "none",
					HideFrom: HideFrom{
						Legend:  false,
						Tooltip: false,
						Viz:     false,
					},
					LineInterpolation: "linear",
					LineWidth:         1,
					PointSize:         5,
					ScaleDistribution: ScaleDistribution{
						Type: "linear",
					},
					ShowPoints: "auto",
					SpanNulls:  false,
					Stacking: Stacking{
						Group: "A",
						Mode:  "none",
					},
					ThresholdsStyle: ThresholdStyle{
						Mode: "off",
					},
				},
				Mappings: nil,
				Thresholds: Thresholds{
					Mode: "absolute",
					Steps: []Steps{{
						Color: "green",
						Value: nil,
					}, {
						Color: "red",
						Value: 80,
					}},
				},
			},
			Overrides: nil,
		},
	}
}

func GenerateDashboard(dbName string) *Dashboard {
	pEventMesh := generateEventMesh()
	pCPULoad := generateCPULoad()
	var panels = []Panels{pEventMesh, pCPULoad}

	if dbName != "" {
		pDBStatus := generateDBStatus(dbName)
		pDBSessions := generateDBSessions(dbName)
		panels = append(panels, pDBSessions)
		panels = append(panels, pDBStatus)
	}

	return &Dashboard{
		Annotations: Annotation{
			List: []List{{
				BuiltIn:    1,
				Datasource: "Prometheus",
				Enable:     true,
				Hide:       true,
				IconColor:  "rgba(0, 211, 255, 1)",
				Limit:      100,
				Name:       "Annotations & Alerts",
				Type:       "dashboard",
			}},
		},
		Description:   "Monitoring",
		Editable:      true,
		GnetID:        0,
		GraphTooltip:  0,
		ID:            0,
		Links:         nil,
		Panels:        panels,
		Refresh:       false,
		SchemaVersion: 30,
		Style:         "dark",
		Tags:          nil,
		Templating: struct {
			List []interface{} `json:"list"`
		}{},
		Time: struct {
			From string `json:"from"`
			To   string `json:"to"`
		}{
			From: "now-1m",
			To:   "now",
		},
		Timepicker: struct {
			RefreshIntervals []string `json:"refresh_intervals"`
			TimeOptions      []string `json:"time_options"`
		}{
			RefreshIntervals: []string{
				"5s",
				"10s",
				"30s",
				"1m",
				"5m",
				"15m",
				"30m",
				"1h",
				"2h",
				"1d",
			},
			TimeOptions: []string{
				"5m",
				"15m",
				"1h",
				"6h",
				"12h",
				"24h",
				"2d",
				"7d",
				"30d",
			},
		},
		Timezone: "browser",
		Title:    "Dashboard",
		UID:      "",
		Version:  1,
	}
}
