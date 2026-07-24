package api

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/asd1asd00000/svm-panel/database"
)

func renderTemplate(name, tpl string, data interface{}) string {
	funcs := template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"safeJS": func(s string) template.JS {
			if strings.TrimSpace(s) == "" {
				return template.JS("null")
			}
			return template.JS(s)
		},
		"safeCSS": func(s string) template.CSS { return template.CSS(s) },
	}
	t, err := template.New(name).Funcs(funcs).Parse(tpl)
	if err != nil {
		return templateErrorPage(err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return templateErrorPage(err)
	}
	return buf.String()
}

func templateErrorPage(err error) string {
	return `<!DOCTYPE html><html lang="fa" dir="rtl"><head><meta charset="UTF-8"><title>Template Error</title></head><body style="font-family:tahoma;background:#0f172a;color:#fff;padding:24px"><h2>خطا در رندر قالب</h2><pre>` +
		template.HTMLEscapeString(err.Error()) +
		`</pre></body></html>`
}

func safeJSON(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	return raw
}

func fallbackText(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func fallbackURL(v string) string {
	if strings.TrimSpace(v) == "" {
		return "#"
	}
	return v
}

func normalizeTab(tab string) string {
	switch tab {
	case "dashboard", "users", "nodes", "settings":
		return tab
	default:
		return "dashboard"
	}
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func safeCSSColor(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return "#10B981"
	}
	if isHexColor(c) {
		return c
	}
	lower := strings.ToLower(c)
	if strings.HasPrefix(lower, "rgb(") || strings.HasPrefix(lower, "rgba(") || strings.HasPrefix(lower, "hsl(") || strings.HasPrefix(lower, "hsla(") {
		if strings.ContainsAny(c, "<>\"'`;{}") {
			return "#10B981"
		}
		return c
	}
	return "#10B981"
}

func isHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	switch len(s) {
	case 4, 5, 7, 9:
	default:
		return false
	}
	for _, r := range s[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// chartRangeOpts is rendered as static HTML (one button per range) so the list
// is always fully clickable (x-for inside a late-init x-show misbehaved in the
// node iframe and only the first item was selectable).
var chartRangeOpts = []struct{ V, L string }{
	{"1h", "۱ ساعت"}, {"2h", "۲ ساعت"}, {"4h", "۴ ساعت"},
	{"6h", "۶ ساعت"}, {"12h", "۱۲ ساعت"}, {"24h", "۲۴ ساعت"},
	{"2d", "۲ روز"}, {"3d", "۳ روز"}, {"5d", "۵ روز"},
	{"7d", "۷ روز"}, {"14d", "۱۴ روز"}, {"30d", "۳۰ روز"},
	{"90d", "۳ ماه"}, {"all", "همه"},
}

// chartRangeDropdownHTML is a self-contained custom range selector. It owns its
// own Alpine scope (svmRangeDropdown), has an opaque (non-blur) background, and
// broadcasts the chosen range via the "svm-range-change" window event so any
// chart component (even in a different scope / iframe) can react.
func chartRangeDropdownHTML() string {
	var b strings.Builder
	b.WriteString(`<div class="relative shrink-0" x-data="svmRangeDropdown()" @click.outside="open=false">`)
	b.WriteString(`<button type="button" @click="open=!open" class="flex items-center gap-2 bg-slate-900 border border-slate-700 rounded-xl sm:rounded-2xl px-3 py-2 text-xs sm:text-sm font-black text-slate-200 outline-none focus:border-cyan-400">`)
	b.WriteString(`<span x-text="label()"></span>`)
	b.WriteString(`<svg class="w-3.5 h-3.5 opacity-70" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>`)
	b.WriteString(`</button>`)
	b.WriteString(`<div x-show="open" x-cloak class="absolute left-0 mt-2 w-44 sm:w-48 max-h-72 overflow-y-auto rounded-2xl border border-slate-700 bg-slate-900 shadow-2xl z-[100] p-1">`)
	for _, o := range chartRangeOpts {
		b.WriteString(`<button type="button" @click="pick('` + o.V + `')" class="w-full flex items-center justify-between gap-2 px-3 py-2.5 rounded-xl text-xs sm:text-sm font-bold text-right" :class="range==='` + o.V + `' ? 'bg-cyan-500/20 text-cyan-300' : 'text-slate-200 hover:bg-slate-800'">`)
		b.WriteString(`<span>` + o.L + `</span>`)
		b.WriteString(`<svg x-show="range==='` + o.V + `'" class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M5 13l4 4L19 7"/></svg>`)
		b.WriteString(`</button>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

func fullscreenBtnHTML() string {
	return `<button type="button" @click="openFullChart()" aria-label="تمام صفحه" class="flex items-center justify-center w-9 h-9 rounded-xl bg-slate-900 border border-slate-700 text-slate-300 active:scale-95">` +
		`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H5a2 2 0 0 0-2 2v3"/><path d="M16 3h3a2 2 0 0 1 2 2v3"/><path d="M8 21H5a2 2 0 0 1-2-2v-3"/><path d="M16 21h3a2 2 0 0 0 2-2v-3"/></svg>` +
		`</button>`
}

func renderLoginHTML() string {
	return renderTemplate("login", `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
	<title>ورود به پنل مدیریت SVM</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<link href="https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;500;700;900&display=swap" rel="stylesheet">
	<style>
		* { box-sizing: border-box; }
		body { font-family: 'Vazirmatn', sans-serif; }
		.mesh {
			background:
				radial-gradient(circle at 15% 20%, rgba(6, 182, 212, .25), transparent 34%),
				radial-gradient(circle at 85% 10%, rgba(139, 92, 246, .18), transparent 30%),
				radial-gradient(circle at 50% 90%, rgba(16, 185, 129, .16), transparent 35%),
				#020617;
		}
		.grid-bg {
			background-image:
				linear-gradient(rgba(148,163,184,.06) 1px, transparent 1px),
				linear-gradient(90deg, rgba(148,163,184,.06) 1px, transparent 1px);
			background-size: 44px 44px;
			mask-image: radial-gradient(circle, black 40%, transparent 78%);
		}
	</style>
</head>
<body class="mesh min-h-screen text-slate-100 flex items-center justify-center p-4 overflow-hidden">
	<div class="fixed inset-0 grid-bg pointer-events-none"></div>

	<main class="relative w-full max-w-md mt-10">
		<div class="absolute -inset-1 rounded-[2rem] bg-gradient-to-l from-cyan-500 via-indigo-500 to-emerald-500 opacity-40 blur-2xl"></div>
		<section class="relative w-full rounded-[2rem] border border-white/10 bg-slate-900/80 backdrop-blur-2xl shadow-2xl p-6 sm:p-9 pt-16 sm:pt-20">
			<div class="absolute left-1/2 -top-12 sm:-top-14 -translate-x-1/2 z-20 flex items-center justify-center">
				<div class="absolute w-28 h-28 sm:w-32 sm:h-32 bg-cyan-400/30 rounded-full blur-2xl"></div>
				<img src="/static/logo.png" alt="svm-panel" class="relative w-28 h-28 sm:w-32 sm:h-32 object-contain drop-shadow-[0_0_25px_rgba(34,211,238,0.5)]" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">
				<div class="relative w-28 h-28 sm:w-32 sm:h-32 items-center justify-center" style="display:none"><span class="text-6xl">⚡</span></div>
			</div>

			<div class="text-center mb-7">
				<h1 class="text-3xl font-black tracking-tight bg-gradient-to-l from-cyan-300 to-emerald-300 bg-clip-text text-transparent">SVM PANEL</h1>
				<p class="text-sm text-slate-400 mt-2">پنل مدیریت کاربران ssh vpn</p>
			</div>

			<form action="/admin/login" method="POST" class="w-full space-y-4">
				<div class="w-full">
					<label class="block text-xs font-bold mb-2 text-slate-300">نام کاربری ادمین</label>
					<input type="text" name="username" required autocomplete="username" class="block w-full max-w-full bg-slate-950/70 border border-slate-700/70 rounded-2xl px-4 py-3.5 text-slate-100 outline-none focus:border-cyan-400 focus:ring-4 focus:ring-cyan-400/10 transition text-left" dir="ltr">
				</div>
				<div class="w-full">
					<label class="block text-xs font-bold mb-2 text-slate-300">رمز عبور</label>
					<input type="password" name="password" required autocomplete="current-password" class="block w-full max-w-full bg-slate-950/70 border border-slate-700/70 rounded-2xl px-4 py-3.5 text-slate-100 outline-none focus:border-cyan-400 focus:ring-4 focus:ring-cyan-400/10 transition text-left" dir="ltr">
				</div>
				<button type="submit" class="block w-full bg-gradient-to-l from-cyan-500 to-emerald-500 hover:from-cyan-400 hover:to-emerald-400 text-slate-950 font-black py-3.5 rounded-[1.75rem] transition shadow-lg shadow-cyan-500/20 active:scale-[.99]">
					ورود به پنل
				</button>
			</form>
		</section>
	</main>
</body>
</html>
`, nil)
}

func renderDrilldownHTML(pageTitle, categoriesJSON, seriesJSON string) string {
	data := struct {
		PageTitle      string
		CategoriesJSON string
		SeriesJSON     string
	}{
		PageTitle:      pageTitle,
		CategoriesJSON: safeJSON(categoriesJSON, "[]"),
		SeriesJSON:     safeJSON(seriesJSON, "[]"),
	}
	return renderTemplate("drilldown", `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
	<title>جزئیات مصرف</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<script src="https://cdn.jsdelivr.net/npm/apexcharts"></script>
	<link href="https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;600;800&display=swap" rel="stylesheet">
	<style>
		body { font-family: 'Vazirmatn', sans-serif; background: transparent; margin:0; height:100vh; overflow:hidden; }
		.apexcharts-tooltip { background:#0f172a !important; border:1px solid #334155 !important; color:#f8fafc !important; box-shadow:0 20px 50px rgba(0,0,0,.35) !important; }
		.apexcharts-tooltip-title { background:#111827 !important; border-bottom:1px solid #334155 !important; font-weight:800 !important; }
	</style>
</head>
<body class="text-slate-100">
	<div class="w-full h-full flex flex-col p-3">
		<div class="shrink-0 mb-3 rounded-2xl border border-white/10 bg-slate-900/70 backdrop-blur px-4 py-3 text-center">
			<h2 class="text-lg md:text-xl font-black bg-gradient-to-l from-cyan-300 to-emerald-300 bg-clip-text text-transparent">📊 {{.PageTitle}}</h2>
		</div>
		<div class="flex-1 min-h-0 rounded-2xl border border-white/10 bg-slate-950/40 p-2">
			<div id="drilldownChart" class="w-full h-full"></div>
		</div>
	</div>
	<script>
		var options = {
			series: {{safeJS .SeriesJSON}},
			chart: { type: "bar", height: "100%", stacked: true, toolbar: { show: false }, fontFamily: "Vazirmatn, Tahoma, sans-serif", foreColor: "#94A3B8", background: "transparent" },
			plotOptions: { bar: { borderRadius: 6, columnWidth: "48%" } },
			dataLabels: { enabled: false },
			xaxis: { categories: {{safeJS .CategoriesJSON}}, axisBorder: { show: false }, axisTicks: { show: false } },
			yaxis: { labels: { formatter: function (val) { return val.toFixed(2) + " GB"; } } },
			grid: { borderColor: "#334155", strokeDashArray: 5 },
			theme: { mode: "dark" },
			colors: ["#22D3EE","#34D399","#FBBF24","#F87171","#A78BFA","#F472B6","#38BDF8"],
			tooltip: { theme: "dark", y: { formatter: function(val) { return val.toFixed(3) + " GB"; } } },
			legend: { position: "top", horizontalAlign: "center", labels: { colors: "#E2E8F0" } }
		};
		new ApexCharts(document.querySelector("#drilldownChart"), options).render();
	</script>
</body>
</html>
`, data)
}

func renderNodeChartHTML(nodeName, chartDataJSON string) string {
	data := struct {
		NodeName      string
		ChartRaw      string
		RangeDropdown string
		CoreScript    string
	}{
		NodeName:      nodeName,
		ChartRaw:      safeJSON(chartDataJSON, `{"hourly":{"categories":[],"series":[]},"daily":{"categories":[],"series":[]}}`),
		RangeDropdown: chartRangeDropdownHTML(),
		CoreScript:    chartCoreScript(),
	}
	return renderTemplate("node-chart", `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
	<title>نمودار ترافیک نود</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<script src="https://cdn.jsdelivr.net/npm/apexcharts"></script>
	<link href="https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;600;800&display=swap" rel="stylesheet">
	<style>
		html,body{height:100%;}
		body { font-family: 'Vazirmatn', sans-serif; background: transparent; margin:0; height:100vh; overflow:hidden; }
		[x-cloak]{display:none !important;}
		.apexcharts-tooltip { background:#0f172a !important; border:1px solid #334155 !important; color:#f8fafc !important; box-shadow:0 20px 50px rgba(0,0,0,.35) !important; }
		.apexcharts-tooltip-title { background:#111827 !important; border-bottom:1px solid #334155 !important; font-weight:800 !important; }
	</style>
	{{safeHTML .CoreScript}}
	<script>window.NODE_RAW = {{safeJS .ChartRaw}};</script>
</head>
<body class="text-slate-100" x-data="svmMakeChartComponent(window.NODE_RAW, 'nodeApexChart')" x-init="mount()">
	<div class="w-full h-full flex flex-col p-3">
		<div class="shrink-0 mb-3 rounded-2xl border border-white/10 bg-slate-900 px-4 py-3 flex items-center justify-between gap-3 relative z-30">
			<h2 class="text-base md:text-lg font-black text-emerald-300 truncate">📊 تفکیک مصرف ترافیک: <span class="text-white">{{.NodeName}}</span></h2>
			{{safeHTML .RangeDropdown}}
		</div>
		<div class="flex-1 min-h-0 rounded-2xl border border-white/10 bg-slate-950/40 p-2 relative z-0">
			<div id="nodeApexChart" class="w-full h-full"></div>
		</div>
		<div class="shrink-0 mt-2 flex flex-wrap items-center justify-between gap-2">
			<p class="text-xs font-black" :class="chartTrendUp === null ? 'text-slate-400' : (chartTrendUp ? 'text-emerald-400' : 'text-rose-400')"><span x-text="chartTrendText"></span></p>
			<p class="text-[11px] text-slate-400">مصرف در این بازه: <span class="text-slate-100 font-black" x-text="chartTotal + ' GB'"></span></p>
		</div>
	</div>
	<script src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
</body>
</html>
`, data)
}

func renderSubHTML(username, barColor string, percent float64, statusBadge, onlineBadge, limitFmt, usedFmt, remFmt, expiryStr, lastSeenStr, annURL, tutURL, configHTML, chartDataJSON string) string {
	data := struct {
		Username      string
		BarColor      string
		Percent       float64
		StatusBadge   string
		OnlineBadge   string
		LimitFmt      string
		UsedFmt       string
		RemFmt        string
		ExpiryStr     string
		LastSeenStr   string
		AnnURL        string
		TutURL        string
		ConfigHTML    string
		ChartRaw      string
		RangeDropdown string
		CoreScript    string
	}{
		Username:      username,
		BarColor:      safeCSSColor(barColor),
		Percent:       clampPercent(percent),
		StatusBadge:   statusBadge,
		OnlineBadge:   onlineBadge,
		LimitFmt:      limitFmt,
		UsedFmt:       usedFmt,
		RemFmt:        remFmt,
		ExpiryStr:     expiryStr,
		LastSeenStr:   lastSeenStr,
		AnnURL:        fallbackURL(annURL),
		TutURL:        fallbackURL(tutURL),
		ConfigHTML:    configHTML,
		ChartRaw:      safeJSON(chartDataJSON, `{"hourly":{"categories":[],"series":[]},"daily":{"categories":[],"series":[]}}`),
		RangeDropdown: chartRangeDropdownHTML(),
		CoreScript:    chartCoreScript(),
	}
	return renderTemplate("subscription", `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
	<title>وضعیت اشتراک | {{.Username}}</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<script src="https://cdn.jsdelivr.net/npm/apexcharts"></script>
	<link href="https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;600;800;900&display=swap" rel="stylesheet">
	<style>
		* { box-sizing: border-box; }
		body { font-family:'Vazirmatn', Tahoma, sans-serif; background:#020617; color:#F8FAFC; margin:0; min-height:100vh; }
		[x-cloak]{display:none !important;}
		.bg-mesh {
			background:
				radial-gradient(circle at 15% 15%, rgba(34,211,238,.20), transparent 33%),
				radial-gradient(circle at 85% 8%, rgba(167,139,250,.16), transparent 30%),
				radial-gradient(circle at 50% 90%, rgba(52,211,153,.14), transparent 36%),
				#020617;
		}
		.glass { background: rgba(15,23,42,.72); backdrop-filter: blur(18px); border:1px solid rgba(255,255,255,.09); box-shadow:0 24px 80px rgba(0,0,0,.35); }
		.info-box { background: rgba(2,6,23,.55); border:1px solid rgba(148,163,184,.18); border-radius:18px; padding:16px; margin-top:16px; }
		.info-box h3 { margin:0 0 14px 0; font-size:15px; color:#E2E8F0; text-align:center; font-weight:800; }
		.config-item { display:flex; justify-content:space-between; align-items:center; gap:12px; background:rgba(15,23,42,.86); border:1px solid rgba(148,163,184,.18); border-radius:14px; padding:12px; margin-bottom:10px; }
		.config-item span { color:#E2E8F0; font-size:13px; word-break:break-word; }
		.config-btn { background:linear-gradient(135deg,#06b6d4,#10b981); color:#020617; border:none; padding:8px 12px; border-radius:10px; cursor:pointer; font-weight:900; font-size:12px; white-space:nowrap; }
		.apexcharts-tooltip { background:#0f172a !important; border:1px solid #334155 !important; color:#f8fafc !important; box-shadow:0 20px 50px rgba(0,0,0,.35) !important; }
		.apexcharts-tooltip-title { background:#111827 !important; border-bottom:1px solid #334155 !important; font-weight:800 !important; }
	</style>
	{{safeHTML .CoreScript}}
	<script>window.SUB_RAW = {{safeJS .ChartRaw}};</script>
</head>
<body class="bg-mesh" x-data="svmMakeChartComponent(window.SUB_RAW, 'apexTrafficChart')" x-init="mount()">
	<main class="w-full min-h-screen flex items-center justify-center p-4">
		<section class="glass w-full max-w-2xl rounded-[2rem] overflow-hidden">
			<div class="p-5 sm:p-7 border-b border-white/10 bg-gradient-to-l from-cyan-500/10 to-emerald-500/10">
				<div class="flex items-center justify-between gap-4">
					<div>
						<p class="text-xs text-slate-400 mb-1">کاربر عزیز</p>
						<h1 class="text-2xl sm:text-3xl font-black tracking-tight">{{.Username}}</h1>
					</div>
					<div class="flex flex-col items-end gap-2">
						{{safeHTML .StatusBadge}}
						{{safeHTML .OnlineBadge}}
					</div>
				</div>
			</div>
			<div class="p-5 sm:p-7 space-y-5">
				<div class="grid grid-cols-2 gap-3">
					<div class="rounded-2xl bg-slate-950/50 border border-white/10 p-4">
						<p class="text-xs text-slate-400">حجم کل دوره</p>
						<p class="font-black text-lg mt-1">{{.LimitFmt}}</p>
					</div>
					<div class="rounded-2xl bg-slate-950/50 border border-white/10 p-4">
						<p class="text-xs text-slate-400">مصرف شده</p>
						<p class="font-black text-lg mt-1">{{.UsedFmt}}</p>
					</div>
					<div class="rounded-2xl bg-slate-950/50 border border-white/10 p-4">
						<p class="text-xs text-slate-400">باقی‌مانده</p>
						<p class="font-black text-lg mt-1 text-emerald-300">{{.RemFmt}}</p>
					</div>
					<div class="rounded-2xl bg-slate-950/50 border border-white/10 p-4">
						<p class="text-xs text-slate-400">انقضا</p>
						<p class="font-black text-sm sm:text-base mt-1 text-rose-300">{{.ExpiryStr}}</p>
					</div>
				</div>
				<div class="rounded-2xl bg-slate-950/50 border border-white/10 p-4">
					<div class="flex justify-between items-center mb-3">
						<span class="text-sm font-bold text-slate-300">آخرین استفاده</span>
						<span class="text-sm font-black">{{.LastSeenStr}}</span>
					</div>
					<div class="h-3 rounded-full overflow-hidden bg-slate-800">
						<div class="h-full rounded-full shadow-lg transition-all duration-500" style="width: {{safeCSS (printf "%.2f%%" .Percent)}}; background: {{safeCSS .BarColor}};"></div>
					</div>
					<p class="text-center text-xs text-slate-400 mt-3">شما {{printf "%.1f" .Percent}}٪ از حجم کل را مصرف کرده‌اید.</p>
				</div>
				<div class="grid grid-cols-2 gap-3">
					<a href="{{.AnnURL}}" target="_blank" class="rounded-2xl border border-rose-400/30 bg-rose-500/10 text-rose-200 hover:bg-rose-500/20 transition p-3 text-center font-black">📢 اطلاعیه‌ها</a>
					<a href="{{.TutURL}}" target="_blank" class="rounded-2xl border border-emerald-400/30 bg-emerald-500/10 text-emerald-200 hover:bg-emerald-500/20 transition p-3 text-center font-black">📚 آموزش‌ها</a>
				</div>
				{{if .ConfigHTML}}
					<div>{{safeHTML .ConfigHTML}}</div>
				{{end}}
				<div class="info-box">
					<div class="flex items-center justify-between gap-2 mb-3 relative z-30">
						<h3 style="margin:0; font-size:14px;">تفکیک مصرف بازه‌ای به نسبت سرورها (GB)</h3>
						{{safeHTML .RangeDropdown}}
					</div>
					<div class="relative z-0">
						<div id="apexTrafficChart" class="min-h-[320px]"></div>
					</div>
					<div class="flex flex-wrap items-center justify-between gap-2 mt-3 pt-3 border-t border-white/5">
						<p class="text-xs font-black" :class="chartTrendUp === null ? 'text-slate-400' : (chartTrendUp ? 'text-emerald-400' : 'text-rose-400')"><span x-text="chartTrendText"></span></p>
						<p class="text-[11px] text-slate-400">مصرف در این بازه: <span class="text-slate-100 font-black" x-text="chartTotal + ' GB'"></span></p>
					</div>
				</div>
			</div>
		</section>
	</main>
	<script>
		function copySingle(text) {
			if (!text) return;
			if (navigator.clipboard) {
				navigator.clipboard.writeText(text).then(function(){ alert("کانفیگ کپی شد!"); });
			} else {
				var ta = document.createElement("textarea");
				ta.value = text;
				document.body.appendChild(ta);
				ta.select();
				document.execCommand("copy");
				ta.remove();
				alert("کانفیگ کپی شد!");
			}
		}
	</script>
	<script src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
</body>
</html>
`, data)
}

type dashboardViewData struct {
	CurrentTab      string
	ToastMsg        string
	ToastIsError    bool
	ChartJSON       string
	BackupColor     string
	BackupText      string
	LogContent      string
	Token           string
	AdminUser       string
	AnnouncementURL string
	TutorialURL     string
	TgBotToken      string
	TgChatID        string
	AutoBackupHours string
	ZipPassword     string
	RangeDropdown   string
	FullscreenBtn   string
	PanelScript     string
	ModalHTML       string
}

func makeDashboardViewData(currentTab, toastMsg string, toastIsError bool, chartJSON, backupColor, backupText, logContent, token, adminUser string) dashboardViewData {
	return dashboardViewData{
		CurrentTab:      normalizeTab(currentTab),
		ToastMsg:        toastMsg,
		ToastIsError:    toastIsError,
		ChartJSON:       safeJSON(chartJSON, "[]"),
		BackupColor:     fallbackText(backupColor, "border-emerald-400/30 bg-emerald-500/10 text-emerald-300"),
		BackupText:      fallbackText(backupText, "فعال"),
		LogContent:      logContent,
		Token:           token,
		AdminUser:       adminUser,
		AnnouncementURL: database.GetSetting("announcement_url"),
		TutorialURL:     database.GetSetting("tutorial_url"),
		TgBotToken:      database.GetSetting("tg_bot_token"),
		TgChatID:        database.GetSetting("tg_chat_id"),
		AutoBackupHours: database.GetSetting("auto_backup_hours"),
		ZipPassword:     database.GetSetting("zip_password"),
		RangeDropdown:   chartRangeDropdownHTML(),
		FullscreenBtn:   fullscreenBtnHTML(),
		PanelScript:     panelDataScript(),
		ModalHTML:       dashboardModalsHTML(),
	}
}

const fullscreenCSS = `
	#dashChartWrap:fullscreen, #dashChartWrap:-webkit-full-screen { background:#020617; padding:16px; display:flex; flex-direction:column; border:none; border-radius:0; }
	#dashChartWrap:fullscreen .fs-chart, #dashChartWrap:-webkit-full-screen .fs-chart { flex:1 1 auto; height:auto !important; min-height:0; }
	#dashChartWrap:fullscreen .fs-head, #dashChartWrap:-webkit-full-screen .fs-head { flex:0 0 auto; }
	#dashChartWrap:fullscreen .fs-foot, #dashChartWrap:-webkit-full-screen .fs-foot { flex:0 0 auto; }
	#fullWrap:fullscreen, #fullWrap:-webkit-full-screen { background:#020617; }
`

func renderMobileDashboard(currentTab, toastMsg string, toastIsError bool, chartJSON, backupColor, backupText, logContent, token, adminUser string) string {
	data := makeDashboardViewData(currentTab, toastMsg, toastIsError, chartJSON, backupColor, backupText, logContent, token, adminUser)
	return renderTemplate("mobile-dashboard", `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
	<title>پنل موبایل SVM</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<script defer src="https://cdn.jsdelivr.net/npm/@alpinejs/collapse@3.x.x/dist/cdn.min.js"></script>
	<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/apexcharts"></script>
	<link href="https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;500;700;800;900&display=swap" rel="stylesheet">
	<style>
		body { font-family:'Vazirmatn', sans-serif; background:#020617; }
		[x-cloak] { display:none !important; }
		.no-scrollbar::-webkit-scrollbar { display:none; }
		.pb-safe { padding-bottom: env(safe-area-inset-bottom); }
		input[type=number]::-webkit-inner-spin-button, input[type=number]::-webkit-outer-spin-button { -webkit-appearance:none; margin:0; }
		input[type=number] { -moz-appearance:textfield; }
		.mesh {
			background:
				radial-gradient(circle at 12% 10%, rgba(34,211,238,.20), transparent 28%),
				radial-gradient(circle at 92% 8%, rgba(167,139,250,.14), transparent 32%),
				radial-gradient(circle at 50% 105%, rgba(52,211,153,.12), transparent 38%),
				#020617;
		}
		.surface { background:rgba(15,23,42,.74); border:1px solid rgba(255,255,255,.08); backdrop-filter:blur(16px); box-shadow:0 18px 60px rgba(0,0,0,.25); }
		.surface-soft { background:rgba(2,6,23,.45); border:1px solid rgba(148,163,184,.14); }
		.apexcharts-tooltip { background:#0f172a !important; border:1px solid #334155 !important; color:#f8fafc !important; }
		.apexcharts-tooltip-title { background:#111827 !important; border-bottom:1px solid #334155 !important; }
		`+fullscreenCSS+`
	</style>
	<script>
		window.SERVER_DATA = {
			currentTab: "{{.CurrentTab}}",
			toastMsg: "{{.ToastMsg}}",
			initialToastIsError: {{.ToastIsError}},
			chartData: {{safeJS .ChartJSON}},
			cpu: 0, ram: 0, allUsers: [], nodes: [], onlineMap: {},
			stats: { totalUsers: 0, onlineUsers: 0, inactiveUsers: 0, totalNodes: 0, activeNodes: 0 }
		};
	</script>
</head>
<body class="mesh text-slate-100 min-h-screen overflow-x-hidden no-scrollbar" x-data="panelData()" x-init="initSetup()" @open-drilldown.window="drilldownUrl = $event.detail; drilldownModal = true">
	<div x-show="showToast" x-cloak :class="toastIsError ? 'bg-rose-500 text-white shadow-rose-500/30' : 'bg-emerald-500 text-slate-950 shadow-emerald-500/20'" class="fixed top-5 left-1/2 -translate-x-1/2 z-[100] rounded-2xl px-5 py-3 font-black shadow-2xl max-w-[90vw] text-sm text-center" x-transition>
		<span x-text="toastMsg"></span>
	</div>
	<header class="sticky top-0 z-40 px-4 py-3 border-b border-white/10 bg-slate-950/70 backdrop-blur-2xl">
		<div class="flex items-center justify-between gap-3">
			<div class="flex items-center gap-2 min-w-0">
								<div class="w-16 h-16 relative shrink-0 flex items-center justify-center">
					<img src="/static/logo.png" alt="SVM PANEL" class="max-w-full max-h-full object-contain mix-blend-screen" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">
										<div class="absolute inset-0 items-center justify-center" style="display:none"><span class="text-3xl">⚡</span></div>
				</div>
				<div class="min-w-0">
					<h1 class="font-black text-base sm:text-lg bg-gradient-to-l from-cyan-300 to-emerald-300 bg-clip-text text-transparent leading-tight truncate">SVM PANEL</h1>
					<p class="text-[9px] sm:text-[10px] text-slate-500 leading-tight truncate">پنل مدیریت کاربران ssh vpn</p>
				</div>
			</div>
			<div x-show="activeTab === 'users'" x-transition class="flex-1 max-w-[190px]">
				<input type="text" x-model="userSearch" @input="currentPage = 1" placeholder="جستجوی کاربر..." class="w-full bg-slate-950/80 border border-slate-700/70 rounded-xl px-3 py-2 text-xs text-slate-100 outline-none focus:border-cyan-400 text-right">
			</div>
			<a href="/admin/logout" class="shrink-0 rounded-xl bg-rose-500/10 border border-rose-400/20 text-rose-300 px-3 py-2 text-xs font-black">خروج</a>
		</div>
	</header>
	<main class="p-4 pb-28">
		<section x-show="activeTab === 'dashboard'" x-transition class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div class="surface rounded-3xl p-4">
					<p class="text-xs text-slate-400">CPU</p>
					<div class="flex items-end justify-between mt-2">
						<span class="text-2xl font-black" x-text="cpu.toFixed(1) + '%'"></span>
						<span class="text-xl">🧠</span>
					</div>
					<div class="h-1.5 bg-slate-800 rounded-full mt-3 overflow-hidden"><div class="h-full bg-cyan-400" :style="'width:' + Math.min(cpu,100) + '%'"></div></div>
				</div>
				<div class="surface rounded-3xl p-4">
					<p class="text-xs text-slate-400">RAM</p>
					<div class="flex items-end justify-between mt-2">
						<span class="text-2xl font-black" x-text="ram.toFixed(1) + '%'"></span>
						<span class="text-xl">💾</span>
					</div>
					<div class="h-1.5 bg-slate-800 rounded-full mt-3 overflow-hidden"><div class="h-full bg-violet-400" :style="'width:' + Math.min(ram,100) + '%'"></div></div>
				</div>
			</div>
			<div class="grid grid-cols-3 gap-2">
				<button @click="activeTab='users'; userFilter='all'; currentPage=1" class="surface rounded-2xl p-3 text-right active:scale-[.99]">
					<p class="text-[10px] text-slate-400">کل کاربران</p>
					<p class="text-xl font-black text-cyan-300 mt-1" x-text="stats.totalUsers"></p>
				</button>
				<button @click="activeTab='users'; userFilter='online'; currentPage=1" class="surface rounded-2xl p-3 text-right active:scale-[.99]">
					<p class="text-[10px] text-slate-400">آنلاین‌ها</p>
					<p class="text-xl font-black text-emerald-300 mt-1" x-text="stats.onlineUsers"></p>
				</button>
				<button @click="activeTab='users'; userFilter='inactive'; currentPage=1" class="surface rounded-2xl p-3 text-right active:scale-[.99]">
					<p class="text-[10px] text-slate-400">غیرفعال</p>
					<p class="text-xl font-black text-rose-300 mt-1" x-text="stats.inactiveUsers"></p>
				</button>
			</div>
			<div id="dashChartWrap" class="surface rounded-3xl p-4">
				<div class="fs-head flex items-center justify-between mb-3 gap-2 relative z-30">
					<div class="min-w-0">
						<h3 class="font-black text-slate-100">📊 مصرف ترافیک</h3>
						<p class="text-[11px] text-slate-500 mt-0.5">مجموع ترافیک همهٔ سرورها در بازهٔ انتخابی</p>
					</div>
					<div class="flex items-center gap-2 shrink-0">
						{{safeHTML .RangeDropdown}}
						{{safeHTML .FullscreenBtn}}
					</div>
				</div>
				<div class="fs-chart h-[260px] relative z-0"><div id="dashboardChart" class="w-full h-full"></div></div>
				<div class="fs-foot mt-3 pt-3 border-t border-white/5">
					<p class="text-xs font-black" :class="chartTrendUp === null ? 'text-slate-400' : (chartTrendUp ? 'text-emerald-400' : 'text-rose-400')">
						<span x-text="chartTrendText"></span>
					</p>
					<p class="text-[11px] text-slate-400 mt-1">مصرف در این بازه: <span class="text-slate-100 font-black" x-text="chartTotal + ' GB'"></span></p>
				</div>
			</div>
			<div class="surface rounded-3xl p-4">
				<div class="flex items-center justify-between mb-3">
					<h3 class="font-black text-slate-100">🖥️ وضعیت نودها</h3>
					<span class="rounded-full bg-cyan-500/10 border border-cyan-400/20 text-cyan-300 text-[11px] font-black px-3 py-1" x-text="stats.activeNodes + '/' + stats.totalNodes + ' فعال'"></span>
				</div>
				<div class="flex flex-wrap gap-2">
					<template x-for="node in nodes" :key="node.IP">
						<div class="surface-soft rounded-2xl px-3 py-2 flex items-center gap-2">
							<span class="w-2.5 h-2.5 rounded-full" :class="node.IsOnline ? 'bg-emerald-400 shadow-[0_0_12px_rgba(52,211,153,.6)]' : 'bg-rose-500'"></span>
							<span class="text-xs font-bold" x-text="node.CustomRemark || node.IP"></span>
						</div>
					</template>
				</div>
			</div>
			<div class="surface rounded-3xl p-4 flex items-center justify-between">
				<div>
					<p class="font-black text-slate-100">اتوبکاپ تلگرام</p>
					<p class="text-xs text-slate-500 mt-1">وضعیت بکاپ‌گیری خودکار</p>
				</div>
				<span class="rounded-full border px-3 py-1 text-xs font-black {{.BackupColor}}">{{.BackupText}}</span>
			</div>
		</section>
		<section x-show="activeTab === 'users'" x-transition class="space-y-3">
			<div class="surface rounded-2xl p-2 grid grid-cols-3 gap-2">
				<button @click="userFilter='all'; currentPage=1" :class="userFilter==='all' ? 'bg-slate-700 text-white' : 'text-slate-400'" class="rounded-2xl py-2 text-xs font-black">همه</button>
				<button @click="userFilter='online'; currentPage=1" :class="userFilter==='online' ? 'bg-emerald-500/15 text-emerald-300' : 'text-slate-400'" class="rounded-2xl py-2 text-xs font-black">آنلاین</button>
				<button @click="userFilter='inactive'; currentPage=1" :class="userFilter==='inactive' ? 'bg-rose-500/15 text-rose-300' : 'text-slate-400'" class="rounded-2xl py-2 text-xs font-black">غیرفعال</button>
			</div>
			<div class="flex items-center gap-2">
				<button @click="prepareCreateUser()" class="flex-1 rounded-2xl bg-gradient-to-l from-cyan-500 to-emerald-500 text-slate-950 font-black py-2.5 shadow-lg shadow-cyan-500/15 active:scale-[.99]">+ کاربر جدید</button>
				<select x-model.number="itemsPerPage" @change="onPerPageChange()" class="shrink-0 bg-slate-950/80 border border-slate-700 rounded-2xl px-2 py-2.5 text-xs font-black text-slate-200 outline-none focus:border-cyan-400">
					<option value="10">10</option>
					<option value="20">20</option>
					<option value="50">50</option>
					<option value="100">100</option>
				</select>
			</div>
			<div class="space-y-2">
				<template x-for="(user, index) in paginatedUsers" :key="user.username">
					<article x-data="{ expanded:false }" class="rounded-2xl overflow-hidden border border-white/10" :style="index % 2 === 0 ? 'background:rgba(255,255,255,0.035)' : 'background:rgba(255,255,255,0.085)'">
						<button @click="expanded=!expanded" class="w-full px-3 py-2.5 text-right">
							<div class="flex items-center justify-between gap-2">
								<div class="flex items-center gap-2.5 min-w-0">
									<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" class="w-5 h-5 shrink-0" :class="isActive(user) ? (onlineMap[user.username] ? 'text-emerald-400 drop-shadow-[0_0_5px_rgba(52,211,153,.7)]' : 'text-white/35') : 'text-rose-400 drop-shadow-[0_0_5px_rgba(251,113,133,.6)]'">
										<path d="M1.5 8.6a18 18 0 0 1 21 0"></path>
										<path d="M5 12.1a13 13 0 0 1 14 0"></path>
										<path d="M8.5 15.6a8 8 0 0 1 7 0"></path>
										<circle cx="12" cy="19.4" r="1.15" fill="currentColor" stroke="none"></circle>
									</svg>
									<div class="min-w-0">
										<p class="font-black text-sm truncate" x-text="user.username"></p>
										<p class="text-[10px] text-slate-500 truncate" x-show="user.comment">💬 یادداشت دارد</p>
									</div>
								</div>
								<div class="text-left shrink-0">
									<p class="font-mono text-xs font-black" x-text="formatBytes(user.data_used || 0)"></p>
									<p class="font-mono text-[10px] text-slate-500" x-text="limitText(user)"></p>
								</div>
							</div>
							<div class="h-1 bg-slate-800 rounded-full overflow-hidden mt-2">
								<div class="h-full rounded-full" :class="isActive(user) ? 'bg-cyan-400' : 'bg-rose-500'" :style="'width:' + usagePercent(user) + '%'"></div>
							</div>
						</button>
						<div x-show="expanded" x-collapse class="px-3 pb-3">
							<div class="surface-soft rounded-2xl p-3 text-xs space-y-2">
								<div class="flex justify-between gap-2"><span class="text-slate-500">انقضا</span><span class="font-bold" x-text="dateFa(user.expiry_unix, false)"></span></div>
								<div class="flex justify-between gap-2"><span class="text-slate-500">باقی‌مانده</span><span class="font-black" :class="daysLeft(user) > 3 ? 'text-emerald-300' : 'text-amber-300'" x-text="daysLeft(user) > 0 ? daysLeft(user) + ' روز' : 'منقضی'"></span></div>
								<div class="flex justify-between gap-2"><span class="text-slate-500">آخرین اتصال</span><span class="font-bold" x-text="dateFa(user.last_seen, false)"></span></div>
								<div x-show="user.comment" class="pt-2 border-t border-slate-700/60">
									<p class="text-slate-500 mb-1">یادداشت</p>
									<p class="font-bold whitespace-pre-wrap" x-text="user.comment"></p>
								</div>
							</div>
							<div class="grid grid-cols-2 gap-2 mt-3">
								<button @click="prepareEditUser(user)" class="rounded-2xl bg-indigo-500/10 text-indigo-300 py-2 text-xs font-black">✏️ ویرایش</button>
								<button @click="copySubLink(user.sub_token)" class="rounded-2xl bg-cyan-500/10 text-cyan-300 py-2 text-xs font-black">🔗 لینک</button>
								<a :href="'/sub/' + user.sub_token" target="_blank" class="rounded-2xl bg-blue-500/10 text-blue-300 py-2 text-xs font-black text-center">👁️ مشاهده</a>
								<form action="/admin/actions" method="POST" onsubmit="return confirm('ریست ترافیک؟')">
									<input type="hidden" name="action" value="reset_traffic">
									<input type="hidden" name="current_tab" value="users">
									<input type="hidden" name="username" :value="user.username">
									<button type="submit" class="w-full rounded-2xl bg-orange-500/10 text-orange-300 py-2 text-xs font-black">🔄 ریست</button>
								</form>
								<form action="/admin/actions" method="POST" onsubmit="return confirm('حذف کاربر؟')" class="col-span-2">
									<input type="hidden" name="action" value="delete_user">
									<input type="hidden" name="current_tab" value="users">
									<input type="hidden" name="username" :value="user.username">
									<button type="submit" class="w-full rounded-2xl bg-rose-500/10 text-rose-300 py-2 text-xs font-black">🗑️ حذف کاربر</button>
								</form>
							</div>
						</div>
					</article>
				</template>
			</div>
			<div class="surface rounded-2xl p-2 flex items-center justify-between">
				<button @click="if(currentPage>1) currentPage--" class="text-cyan-300 font-black px-4 py-2">قبلی</button>
				<span class="text-xs font-black" x-text="currentPage + ' / ' + totalPages"></span>
				<button @click="if(currentPage<totalPages) currentPage++" class="text-cyan-300 font-black px-4 py-2">بعدی</button>
			</div>
		</section>
		<section x-show="activeTab === 'nodes'" x-transition class="space-y-3">
			<template x-for="node in nodes" :key="node.IP">
				<article x-data="{ expanded:false }" class="surface rounded-3xl overflow-hidden">
					<button @click="expanded=!expanded" class="w-full p-4 text-right flex items-center justify-between gap-3">
						<div class="flex items-center gap-3 min-w-0">
							<span class="text-xl" x-text="node.IsOnline ? '🟢' : '🔴'"></span>
							<div class="min-w-0">
								<p class="font-black truncate" x-text="node.CustomRemark || node.IP"></p>
								<p class="font-mono text-[10px] text-slate-500 truncate" x-text="node.IP"></p>
							</div>
						</div>
						<span class="font-mono text-xs font-black shrink-0" x-text="formatGB(node.TotalTraffic || 0)"></span>
					</button>
					<div x-show="expanded" x-collapse class="px-4 pb-4">
						<div class="surface-soft rounded-2xl p-3 text-xs space-y-2 mb-3">
							<div class="flex justify-between"><span class="text-slate-500">دامنه</span><span class="font-mono" x-text="node.Domain || 'ندارد'"></span></div>
							<div class="flex justify-between"><span class="text-slate-500">آخرین پینگ</span><span x-text="dateFa(node.LastSeen, true)"></span></div>
						</div>
						<div class="grid grid-cols-2 gap-2">
							<button @click="window.dispatchEvent(new CustomEvent('open-drilldown', { detail: '/admin/node-chart?ip=' + node.IP }))" class="rounded-2xl bg-emerald-500/10 text-emerald-300 py-2 text-xs font-black">📊 نمودار</button>
							<button @click="prepareNodeEdit(node)" class="rounded-2xl bg-indigo-500/10 text-indigo-300 py-2 text-xs font-black">✏️ ویرایش</button>
						</div>
					</div>
				</article>
			</template>
		</section>
		<section x-show="activeTab === 'settings'" x-transition class="space-y-4">
			<div class="surface rounded-3xl p-4">
				<h3 class="font-black text-cyan-300 mb-3">توکن کلاستر</h3>
				<input type="text" readonly value="{{.Token}}" dir="ltr" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-xs font-mono text-left outline-none mb-3">
				<button type="button" data-token="{{.Token}}" onclick="copyText(this.dataset.token)" class="w-full rounded-2xl bg-cyan-500 text-slate-950 py-3 font-black">کپی توکن</button>
			</div>
			<form action="/admin/actions" method="POST" class="surface rounded-3xl p-4 space-y-3">
				<input type="hidden" name="action" value="change_credentials">
				<input type="hidden" name="current_tab" value="settings">
				<h3 class="font-black text-amber-300">تغییر ورود پنل</h3>
				<input type="text" name="admin_username" value="{{.AdminUser}}" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<input type="password" name="admin_password" placeholder="رمز جدید..." class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<button type="submit" class="w-full rounded-2xl bg-amber-500 text-slate-950 py-3 font-black">ذخیره</button>
			</form>
			<div class="surface rounded-3xl p-4 space-y-3">
				<h3 class="font-black text-emerald-300">بکاپ دیتابیس</h3>
				<a href="/admin/backup/download" class="block text-center rounded-2xl bg-emerald-500 text-slate-950 font-black py-3">دانلود بکاپ</a>
				<form action="/admin/backup/restore" method="POST" enctype="multipart/form-data" class="space-y-3 pt-3 border-t border-white/10">
					<input type="file" name="backup_file" accept=".sql" required class="text-xs text-slate-300 w-full">
					<button type="submit" class="w-full rounded-2xl bg-orange-500 text-slate-950 font-black py-3">بازگردانی</button>
				</form>
			</div>
			<form action="/admin/actions" method="POST" class="surface rounded-3xl p-4 space-y-3">
				<input type="hidden" name="action" value="update_settings">
				<input type="hidden" name="current_tab" value="settings">
				<h3 class="font-black text-cyan-300">لینک‌ها و تلگرام</h3>
				<input type="text" name="announcement_url" value="{{.AnnouncementURL}}" placeholder="لینک اطلاعیه" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<input type="text" name="tutorial_url" value="{{.TutorialURL}}" placeholder="لینک آموزش" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<input type="text" name="tg_bot_token" value="{{.TgBotToken}}" placeholder="توکن ربات" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<input type="text" name="tg_chat_id" value="{{.TgChatID}}" placeholder="Chat ID" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<input type="number" name="auto_backup_hours" value="{{.AutoBackupHours}}" placeholder="ساعت بکاپ" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<input type="text" name="zip_password" value="{{.ZipPassword}}" placeholder="رمز فایل زیپ بکاپ" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-3 py-3 text-sm outline-none">
				<button type="submit" class="w-full rounded-2xl bg-cyan-500 text-slate-950 py-3 font-black">ذخیره تنظیمات</button>
			</form>
		</section>
	</main>
	<nav class="fixed bottom-0 left-0 right-0 z-50 h-[76px] pb-safe bg-slate-950/85 backdrop-blur-2xl border-t border-white/10 grid grid-cols-4">
		<button @click="activeTab='dashboard'; window.scrollTo(0,0)" class="flex flex-col items-center justify-center gap-1 transition" :class="activeTab==='dashboard' ? 'text-cyan-300 scale-105' : 'text-slate-500'"><span class="text-xl">📊</span><span class="text-[10px] font-black">داشبورد</span></button>
		<button @click="activeTab='users'; window.scrollTo(0,0)" class="flex flex-col items-center justify-center gap-1 transition" :class="activeTab==='users' ? 'text-cyan-300 scale-105' : 'text-slate-500'"><span class="text-xl">👥</span><span class="text-[10px] font-black">کاربران</span></button>
		<button @click="activeTab='nodes'; window.scrollTo(0,0)" class="flex flex-col items-center justify-center gap-1 transition" :class="activeTab==='nodes' ? 'text-cyan-300 scale-105' : 'text-slate-500'"><span class="text-xl">🖥️</span><span class="text-[10px] font-black">نودها</span></button>
		<button @click="activeTab='settings'; window.scrollTo(0,0)" class="flex flex-col items-center justify-center gap-1 transition" :class="activeTab==='settings' ? 'text-cyan-300 scale-105' : 'text-slate-500'"><span class="text-xl">⚙️</span><span class="text-[10px] font-black">تنظیمات</span></button>
	</nav>
	{{safeHTML .ModalHTML}}
	{{safeHTML .PanelScript}}
</body>
</html>
`, data)
}

func renderDesktopDashboard(currentTab, toastMsg string, toastIsError bool, chartJSON, backupColor, backupText, logContent, token, adminUser string) string {
	data := makeDashboardViewData(currentTab, toastMsg, toastIsError, chartJSON, backupColor, backupText, logContent, token, adminUser)
	return renderTemplate("desktop-dashboard", `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
	<title>داشبورد SVM</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<script defer src="https://cdn.jsdelivr.net/npm/@alpinejs/collapse@3.x.x/dist/cdn.min.js"></script>
	<script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/apexcharts"></script>
	<link href="https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;500;700;800;900&display=swap" rel="stylesheet">
	<style>
		body { font-family:'Vazirmatn', sans-serif; background:#020617; }
		[x-cloak] { display:none !important; }
		::-webkit-scrollbar { width:7px; height:7px; }
		::-webkit-scrollbar-track { background:#020617; }
		::-webkit-scrollbar-thumb { background:#334155; border-radius:99px; }
		input[type=number]::-webkit-inner-spin-button, input[type=number]::-webkit-outer-spin-button { -webkit-appearance:none; margin:0; }
		input[type=number] { -moz-appearance:textfield; }
		.mesh {
			background:
				radial-gradient(circle at 10% 5%, rgba(34,211,238,.18), transparent 28%),
				radial-gradient(circle at 88% 10%, rgba(167,139,250,.14), transparent 32%),
				radial-gradient(circle at 50% 100%, rgba(52,211,153,.10), transparent 40%),
				#020617;
		}
		.surface { background:rgba(15,23,42,.72); border:1px solid rgba(255,255,255,.08); backdrop-filter:blur(16px); box-shadow:0 20px 70px rgba(0,0,0,.25); }
		.surface-soft { background:rgba(2,6,23,.45); border:1px solid rgba(148,163,184,.14); }
		.nav-btn { width:100%; display:flex; align-items:center; gap:12px; padding:14px 16px; border-radius:18px; font-weight:900; transition:.2s; }
		.apexcharts-tooltip { background:#0f172a !important; border:1px solid #334155 !important; color:#f8fafc !important; }
		.apexcharts-tooltip-title { background:#111827 !important; border-bottom:1px solid #334155 !important; }
		`+fullscreenCSS+`
	</style>
	<script>
		window.SERVER_DATA = {
			currentTab: "{{.CurrentTab}}",
			toastMsg: "{{.ToastMsg}}",
			initialToastIsError: {{.ToastIsError}},
			chartData: {{safeJS .ChartJSON}},
			cpu: 0, ram: 0, allUsers: [], nodes: [], onlineMap: {},
			stats: { totalUsers: 0, onlineUsers: 0, inactiveUsers: 0, totalNodes: 0, activeNodes: 0 }
		};
	</script>
</head>
<body class="mesh text-slate-100 min-h-screen overflow-x-hidden" x-data="panelData()" x-init="initSetup()" @open-drilldown.window="drilldownUrl = $event.detail; drilldownModal = true">
	<div x-show="showToast" x-cloak :class="toastIsError ? 'bg-rose-500 text-white shadow-rose-500/30' : 'bg-emerald-500 text-slate-950 shadow-emerald-500/20'" class="fixed bottom-6 left-6 z-[100] rounded-2xl px-6 py-4 font-black shadow-2xl" x-transition>
		<span x-text="toastMsg"></span>
	</div>
	<aside class="fixed right-0 top-0 h-screen w-72 z-50 bg-slate-950/78 backdrop-blur-2xl border-l border-white/10 flex flex-col">
		<div class="px-4 pt-5 pb-4 border-b border-white/10">
			<div class="flex flex-col items-center gap-2">
				<div class="w-full flex items-center justify-center">
					<img src="/static/logo.png" alt="SVM PANEL" class="w-full h-auto max-h-40 object-contain mix-blend-screen" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">
					<div class="w-full h-24 items-center justify-center" style="display:none"><span class="text-5xl">⚡</span></div>
				</div>
				<div class="text-center">
					<h1 class="text-xl font-black bg-gradient-to-l from-cyan-300 to-emerald-300 bg-clip-text text-transparent">SVM PANEL</h1>
					<p class="text-[11px] text-slate-500 mt-1">پنل مدیریت کاربران ssh vpn</p>
				</div>
			</div>
		</div>
		<nav class="flex-1 p-4 space-y-2 overflow-y-auto">
			<button @click="activeTab='dashboard'" class="nav-btn" :class="activeTab==='dashboard' ? 'bg-cyan-500 text-slate-950 shadow-lg shadow-cyan-500/20' : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'">📊 داشبورد اصلی</button>
			<button @click="activeTab='users'" class="nav-btn" :class="activeTab==='users' ? 'bg-cyan-500 text-slate-950 shadow-lg shadow-cyan-500/20' : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'">👥 مدیریت کاربران</button>
			<button @click="activeTab='nodes'" class="nav-btn" :class="activeTab==='nodes' ? 'bg-cyan-500 text-slate-950 shadow-lg shadow-cyan-500/20' : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'">🖥️ نودهای متصل</button>
			<button @click="activeTab='settings'" class="nav-btn" :class="activeTab==='settings' ? 'bg-cyan-500 text-slate-950 shadow-lg shadow-cyan-500/20' : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'">⚙️ تنظیمات سیستم</button>
		</nav>
		<div class="p-4 border-t border-white/10">
			<a href="/admin/logout" class="block text-center rounded-2xl bg-rose-500/10 border border-rose-400/20 text-rose-300 hover:bg-rose-500 hover:text-white transition py-3 font-black">🚪 خروج</a>
		</div>
	</aside>
	<div class="pr-72 min-h-screen">
		<header class="sticky top-0 z-40 bg-slate-950/60 backdrop-blur-2xl border-b border-white/10 px-8 py-4 flex items-center justify-between gap-5">
			<div>
				<h2 class="text-2xl font-black" x-text="activeTab === 'dashboard' ? 'داشبورد' : (activeTab === 'users' ? 'کاربران' : (activeTab === 'nodes' ? 'نودها' : 'تنظیمات'))"></h2>
				<p class="text-xs text-slate-500 mt-1">پنل مدیریت مدرن SVM</p>
			</div>
			<div x-show="activeTab === 'users'" x-transition class="w-[420px]">
				<input type="text" x-model="userSearch" @input="currentPage = 1" placeholder="🔍 جستجوی نام کاربری یا یادداشت..." class="w-full bg-slate-950/80 border border-slate-700 rounded-2xl px-4 py-3 text-sm outline-none focus:border-cyan-400 text-right">
			</div>
		</header>
		<main class="p-8">
			<section x-show="activeTab === 'dashboard'" x-transition class="space-y-6">
				<div class="grid grid-cols-5 gap-5">
					<div class="surface rounded-3xl p-5">
						<p class="text-sm text-slate-400">پردازنده</p>
						<div class="flex items-end justify-between mt-3"><span class="text-3xl font-black" x-text="cpu.toFixed(1) + '%'"></span><span class="text-2xl">🧠</span></div>
						<div class="h-2 bg-slate-800 rounded-full mt-4 overflow-hidden"><div class="h-full bg-cyan-400" :style="'width:' + Math.min(cpu,100) + '%'"></div></div>
					</div>
					<div class="surface rounded-3xl p-5">
						<p class="text-sm text-slate-400">رم</p>
						<div class="flex items-end justify-between mt-3"><span class="text-3xl font-black" x-text="ram.toFixed(1) + '%'"></span><span class="text-2xl">💾</span></div>
						<div class="h-2 bg-slate-800 rounded-full mt-4 overflow-hidden"><div class="h-full bg-violet-400" :style="'width:' + Math.min(ram,100) + '%'"></div></div>
					</div>
					<button @click="activeTab='users'; userFilter='all'; currentPage=1" class="surface rounded-3xl p-5 text-right hover:-translate-y-0.5 transition">
						<p class="text-sm text-slate-400">کل کاربران</p>
						<p class="text-3xl font-black text-cyan-300 mt-3" x-text="stats.totalUsers"></p>
					</button>
					<button @click="activeTab='users'; userFilter='online'; currentPage=1" class="surface rounded-3xl p-5 text-right hover:-translate-y-0.5 transition">
						<p class="text-sm text-slate-400">آنلاین</p>
						<p class="text-3xl font-black text-emerald-300 mt-3" x-text="stats.onlineUsers"></p>
					</button>
					<button @click="activeTab='users'; userFilter='inactive'; currentPage=1" class="surface rounded-3xl p-5 text-right hover:-translate-y-0.5 transition">
						<p class="text-sm text-slate-400">غیرفعال</p>
						<p class="text-3xl font-black text-rose-300 mt-3" x-text="stats.inactiveUsers"></p>
					</button>
				</div>
				<div class="surface rounded-3xl p-5">
					<div class="flex items-center justify-between mb-4">
						<div>
							<h3 class="text-xl font-black">🖥️ نودهای متصل</h3>
							<p class="text-xs text-slate-500 mt-1">وضعیت لحظه‌ای سرورها</p>
						</div>
						<span class="rounded-full bg-cyan-500/10 border border-cyan-400/20 text-cyan-300 text-xs font-black px-4 py-2" x-text="stats.activeNodes + '/' + stats.totalNodes + ' فعال'"></span>
					</div>
					<div class="flex flex-wrap gap-2">
						<template x-for="node in nodes" :key="node.IP">
							<div class="surface-soft rounded-2xl px-4 py-2 flex items-center gap-2">
								<span class="w-2.5 h-2.5 rounded-full" :class="node.IsOnline ? 'bg-emerald-400 shadow-[0_0_12px_rgba(52,211,153,.7)]' : 'bg-rose-500'"></span>
								<span class="text-xs font-black" x-text="node.CustomRemark || node.IP"></span>
							</div>
						</template>
					</div>
				</div>
				<div class="surface rounded-3xl p-5 flex items-center justify-between">
					<div class="flex items-center gap-3">
						<div class="w-12 h-12 rounded-2xl bg-cyan-400/10 border border-cyan-300/20 flex items-center justify-center">🤖</div>
						<div>
							<p class="font-black">وضعیت اتوبکاپ ربات تلگرام</p>
							<p class="text-xs text-slate-500 mt-1">پشتیبان‌گیری خودکار دیتابیس</p>
						</div>
					</div>
					<span class="rounded-full border px-4 py-2 text-xs font-black {{.BackupColor}}">{{.BackupText}}</span>
				</div>
				<div id="dashChartWrap" class="surface rounded-3xl p-6">
					<div class="fs-head flex items-start justify-between mb-4 gap-4 relative z-30">
						<div class="min-w-0">
							<h3 class="text-xl font-black">📊 نمودار مصرف ترافیک کلاستر</h3>
							<p class="text-xs text-slate-500 mt-2">مجموع ترافیک همهٔ سرورها در بازهٔ انتخابی</p>
						</div>
						<div class="flex items-center gap-2 shrink-0">
							{{safeHTML .RangeDropdown}}
							{{safeHTML .FullscreenBtn}}
						</div>
					</div>
					<div class="fs-chart h-[400px] relative z-0"><div id="dashboardChart" class="w-full h-full"></div></div>
					<div class="fs-foot mt-4 pt-4 border-t border-white/5 flex flex-wrap items-center justify-between gap-3">
						<p class="text-sm font-black" :class="chartTrendUp === null ? 'text-slate-400' : (chartTrendUp ? 'text-emerald-400' : 'text-rose-400')">
							<span x-text="chartTrendText"></span>
						</p>
						<p class="text-xs text-slate-400">مصرف در این بازه: <span class="text-slate-100 font-black" x-text="chartTotal + ' GB'"></span> &nbsp;•&nbsp; مجموع ترافیک همهٔ سرورها</p>
					</div>
				</div>
				<div class="surface rounded-3xl p-6">
					<h3 class="text-xl font-black mb-4">📜 لاگ‌های سیستم</h3>
					<pre class="bg-slate-950/70 border border-slate-800 text-xs text-slate-300 p-4 rounded-2xl max-h-96 overflow-y-auto whitespace-pre-wrap break-words font-mono text-left" dir="ltr">{{.LogContent}}</pre>
				</div>
			</section>
			<section x-show="activeTab === 'users'" x-transition class="space-y-5">
				<div class="surface rounded-3xl p-3 flex items-center justify-between gap-4">
					<div class="flex gap-2">
						<button @click="userFilter='all'; currentPage=1" :class="userFilter==='all' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-white'" class="rounded-2xl px-5 py-2 text-sm font-black">همه</button>
						<button @click="userFilter='online'; currentPage=1" :class="userFilter==='online' ? 'bg-emerald-500/15 text-emerald-300' : 'text-slate-400 hover:text-emerald-300'" class="rounded-2xl px-5 py-2 text-sm font-black">آنلاین</button>
						<button @click="userFilter='inactive'; currentPage=1" :class="userFilter==='inactive' ? 'bg-rose-500/15 text-rose-300' : 'text-slate-400 hover:text-rose-300'" class="rounded-2xl px-5 py-2 text-sm font-black">غیرفعال</button>
					</div>
					<div class="flex items-center gap-3">
						<select x-model.number="itemsPerPage" @change="onPerPageChange()" class="bg-slate-950/80 border border-slate-700 rounded-2xl px-3 py-2 text-sm outline-none">
							<option value="10">10</option>
							<option value="20">20</option>
							<option value="50">50</option>
							<option value="100">100</option>
						</select>
						<button @click="prepareCreateUser()" class="rounded-2xl bg-gradient-to-l from-cyan-500 to-emerald-500 text-slate-950 px-6 py-2.5 font-black shadow-lg shadow-cyan-500/15">+ کاربر جدید</button>
					</div>
				</div>
				<div class="surface rounded-3xl overflow-hidden">
					<div class="grid grid-cols-12 gap-3 px-6 py-4 bg-slate-950/70 border-b border-white/10 text-xs font-black text-slate-400">
						<button @click="sortBy('username')" class="col-span-4 text-right hover:text-cyan-300">نام کاربری <span x-text="sortIcon('username')"></span></button>
						<button @click="sortBy('status')" class="col-span-2 text-center hover:text-cyan-300">وضعیت <span x-text="sortIcon('status')"></span></button>
						<button @click="sortBy('data_used')" class="col-span-3 text-left hover:text-cyan-300">مصرف <span x-text="sortIcon('data_used')"></span></button>
						<button @click="sortBy('expiry')" class="col-span-3 text-left hover:text-cyan-300">انقضا <span x-text="sortIcon('expiry')"></span></button>
					</div>
					<div class="divide-y divide-white/10">
						<template x-for="user in paginatedUsers" :key="user.username">
							<article x-data="{ expanded:false }" class="hover:bg-white/[.03] transition">
								<button @click="expanded=!expanded" class="w-full grid grid-cols-12 gap-3 items-center px-6 py-4 text-right">
									<div class="col-span-4 flex items-center gap-3 min-w-0">
										<span class="text-xl" x-text="isActive(user) ? (onlineMap[user.username] ? '🟢' : '⚪') : '🔴'"></span>
										<div class="min-w-0">
											<p class="font-black truncate" x-text="user.username"></p>
											<p class="text-xs text-slate-500 mt-1 truncate" x-show="user.comment">💬 یادداشت دارد</p>
										</div>
									</div>
									<div class="col-span-2 text-center">
										<span class="rounded-xl px-3 py-1 text-xs font-black" :class="isActive(user) ? 'bg-emerald-500/10 text-emerald-300' : 'bg-rose-500/10 text-rose-300'" x-text="isActive(user) ? 'فعال' : 'منقضی'"></span>
									</div>
									<div class="col-span-3 text-left">
										<p class="font-mono font-black" x-text="formatBytes(user.data_used || 0)"></p>
										<p class="font-mono text-xs text-slate-500" x-text="limitText(user)"></p>
									</div>
									<div class="col-span-3 text-left">
										<p class="font-black" x-text="daysLeft(user) > 0 ? daysLeft(user) + ' روز' : 'منقضی'"></p>
										<p class="text-xs text-slate-500" x-text="dateFa(user.expiry_unix, false)"></p>
									</div>
								</button>
								<div x-show="expanded" x-collapse class="px-6 pb-6">
									<div class="h-2 bg-slate-800 rounded-full overflow-hidden mb-4">
										<div class="h-full rounded-full" :class="isActive(user) ? 'bg-cyan-400' : 'bg-rose-500'" :style="'width:' + usagePercent(user) + '%'"></div>
									</div>
									<div class="surface-soft rounded-2xl p-4 grid grid-cols-2 gap-4 text-sm mb-4">
										<div><span class="text-slate-500 ml-2">آخرین اتصال:</span><span class="font-bold" x-text="dateFa(user.last_seen, true)"></span></div>
										<div><span class="text-slate-500 ml-2">توکن:</span><span class="font-mono text-xs" x-text="user.sub_token"></span></div>
										<div x-show="user.comment" class="col-span-2 border-t border-slate-700/60 pt-3">
											<span class="text-slate-500 ml-2">یادداشت:</span>
											<span class="font-bold whitespace-pre-wrap" x-text="user.comment"></span>
										</div>
									</div>
									<div class="flex items-center gap-3">
										<button @click="prepareEditUser(user)" class="rounded-2xl bg-indigo-500/10 text-indigo-300 hover:bg-indigo-500 hover:text-white px-4 py-2 text-sm font-black transition">✏️ ویرایش</button>
										<a :href="'/sub/' + user.sub_token" target="_blank" class="rounded-2xl bg-blue-500/10 text-blue-300 hover:bg-blue-500 hover:text-white px-4 py-2 text-sm font-black transition">👁️ مشاهده</a>
										<button @click="copySubLink(user.sub_token)" class="rounded-2xl bg-cyan-500/10 text-cyan-300 hover:bg-cyan-500 hover:text-slate-950 px-4 py-2 text-sm font-black transition">🔗 کپی لینک</button>
										<div class="flex-1"></div>
										<form action="/admin/actions" method="POST" onsubmit="return confirm('ریست ترافیک؟')">
											<input type="hidden" name="action" value="reset_traffic">
											<input type="hidden" name="current_tab" value="users">
											<input type="hidden" name="username" :value="user.username">
											<button type="submit" class="rounded-2xl bg-orange-500/10 text-orange-300 hover:bg-orange-500 hover:text-white px-4 py-2 text-sm font-black transition">🔄 ریست</button>
										</form>
										<form action="/admin/actions" method="POST" onsubmit="return confirm('حذف کاربر؟')">
											<input type="hidden" name="action" value="delete_user">
											<input type="hidden" name="current_tab" value="users">
											<input type="hidden" name="username" :value="user.username">
											<button type="submit" class="rounded-2xl bg-rose-500/10 text-rose-300 hover:bg-rose-500 hover:text-white px-4 py-2 text-sm font-black transition">🗑️ حذف</button>
										</form>
									</div>
								</div>
							</article>
						</template>
					</div>
					<div class="bg-slate-950/70 border-t border-white/10 p-4 flex items-center justify-between">
						<button @click="if(currentPage>1) currentPage--" class="rounded-2xl px-5 py-2 text-cyan-300 hover:bg-white/5 font-black">قبلی</button>
						<span class="text-sm font-black" x-text="'صفحه ' + currentPage + ' از ' + totalPages"></span>
						<button @click="if(currentPage<totalPages) currentPage++" class="rounded-2xl px-5 py-2 text-cyan-300 hover:bg-white/5 font-black">بعدی</button>
					</div>
				</div>
			</section>
			<section x-show="activeTab === 'nodes'" x-transition class="space-y-5">
				<div class="surface rounded-3xl overflow-hidden">
					<div class="grid grid-cols-12 gap-3 px-6 py-4 bg-slate-950/70 border-b border-white/10 text-xs font-black text-slate-400">
						<div class="col-span-5">سرور نود</div>
						<div class="col-span-2 text-center">وضعیت</div>
						<div class="col-span-3 text-left">ترافیک کل</div>
						<div class="col-span-2 text-left">آخرین پینگ</div>
					</div>
					<div class="divide-y divide-white/10">
						<template x-for="node in nodes" :key="node.IP">
							<article x-data="{ expanded:false }" class="hover:bg-white/[.03] transition">
								<button @click="expanded=!expanded" class="w-full grid grid-cols-12 gap-3 items-center px-6 py-4 text-right">
									<div class="col-span-5 flex items-center gap-3 min-w-0">
										<span class="text-2xl" x-text="node.IsOnline ? '🟢' : '🔴'"></span>
										<div class="min-w-0">
											<p class="font-black truncate" x-text="node.CustomRemark || node.IP"></p>
											<p class="font-mono text-xs text-slate-500 truncate" x-text="node.IP"></p>
										</div>
									</div>
									<div class="col-span-2 text-center">
										<span class="rounded-xl px-3 py-1 text-xs font-black" :class="node.IsOnline ? 'bg-emerald-500/10 text-emerald-300' : 'bg-rose-500/10 text-rose-300'" x-text="node.IsOnline ? 'آنلاین' : 'آفلاین'"></span>
									</div>
									<div class="col-span-3 text-left font-mono font-black" x-text="formatGB(node.TotalTraffic || 0)"></div>
									<div class="col-span-2 text-left text-xs text-slate-400" x-text="dateFa(node.LastSeen, true)"></div>
								</button>
								<div x-show="expanded" x-collapse class="px-6 pb-6">
									<div class="surface-soft rounded-2xl p-4 grid grid-cols-2 gap-4 text-sm mb-4">
										<div><span class="text-slate-500 ml-2">دامنه:</span><span class="font-mono" x-text="node.Domain || 'ندارد'"></span></div>
										<div><span class="text-slate-500 ml-2">آی‌پی:</span><span class="font-mono" x-text="node.IP"></span></div>
									</div>
									<div class="flex gap-3">
										<button @click="window.dispatchEvent(new CustomEvent('open-drilldown', { detail: '/admin/node-chart?ip=' + node.IP }))" class="rounded-2xl bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500 hover:text-slate-950 px-5 py-2 text-sm font-black transition">📊 نمودار</button>
										<button @click="prepareNodeEdit(node)" class="rounded-2xl bg-indigo-500/10 text-indigo-300 hover:bg-indigo-500 hover:text-white px-5 py-2 text-sm font-black transition">✏️ ویرایش</button>
									</div>
								</div>
							</article>
						</template>
					</div>
				</div>
			</section>
			<section x-show="activeTab === 'settings'" x-transition class="space-y-6">
				<div class="grid grid-cols-2 gap-6">
					<div class="surface rounded-3xl p-6">
						<h3 class="text-xl font-black text-cyan-300 mb-4">توکن کلاستر</h3>
						<div class="flex gap-3">
							<input type="text" readonly value="{{.Token}}" dir="ltr" class="flex-1 bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 font-mono text-left outline-none">
							<button type="button" data-token="{{.Token}}" onclick="copyText(this.dataset.token)" class="rounded-2xl bg-cyan-500 text-slate-950 px-6 font-black">کپی</button>
						</div>
					</div>
					<div class="surface rounded-3xl p-6">
						<h3 class="text-xl font-black text-emerald-300 mb-4">بکاپ لوکال</h3>
						<div class="flex items-center gap-4">
							<a href="/admin/backup/download" class="flex-1 text-center rounded-2xl bg-emerald-500 text-slate-950 font-black py-3">دانلود</a>
							<form action="/admin/backup/restore" method="POST" enctype="multipart/form-data" class="flex-1 flex items-center gap-3">
								<input type="file" name="backup_file" accept=".sql" required class="text-xs text-slate-300 w-full">
								<button type="submit" class="rounded-2xl bg-orange-500 text-slate-950 font-black px-5 py-3">اجرا</button>
							</form>
						</div>
					</div>
				</div>
				<form action="/admin/actions" method="POST" class="surface rounded-3xl p-6">
					<input type="hidden" name="action" value="change_credentials">
					<input type="hidden" name="current_tab" value="settings">
					<h3 class="text-xl font-black text-amber-300 mb-4">تغییر رمز ورود</h3>
					<div class="grid grid-cols-3 gap-4 items-end">
						<div>
							<label class="block text-sm mb-2 text-slate-400">نام کاربری</label>
							<input type="text" name="admin_username" value="{{.AdminUser}}" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none">
						</div>
						<div>
							<label class="block text-sm mb-2 text-slate-400">رمز جدید</label>
							<input type="password" name="admin_password" placeholder="در صورت نیاز..." class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none">
						</div>
						<button type="submit" class="rounded-2xl bg-amber-500 text-slate-950 font-black py-3">ذخیره رمز</button>
					</div>
				</form>
				<form action="/admin/actions" method="POST" class="surface rounded-3xl p-6">
					<input type="hidden" name="action" value="update_settings">
					<input type="hidden" name="current_tab" value="settings">
					<h3 class="text-xl font-black text-cyan-300 mb-4">لینک‌ها و تلگرام</h3>
					<div class="grid grid-cols-2 gap-4">
						<div><label class="block text-sm mb-2 text-slate-400">لینک اطلاعیه</label><input type="text" name="announcement_url" value="{{.AnnouncementURL}}" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-sm mb-2 text-slate-400">لینک آموزش</label><input type="text" name="tutorial_url" value="{{.TutorialURL}}" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-sm mb-2 text-slate-400">توکن ربات</label><input type="text" name="tg_bot_token" value="{{.TgBotToken}}" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-sm mb-2 text-slate-400">Chat ID</label><input type="text" name="tg_chat_id" value="{{.TgChatID}}" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-sm mb-2 text-slate-400">ساعت بکاپ</label><input type="number" name="auto_backup_hours" value="{{.AutoBackupHours}}" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-sm mb-2 text-slate-400">رمز فایل زیپ بکاپ</label><input type="text" name="zip_password" value="{{.ZipPassword}}" placeholder="در صورت خالی بودن رمز نمیخورد" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
					</div>
					<button type="submit" class="mt-6 rounded-2xl bg-cyan-500 text-slate-950 font-black py-3 px-8">ذخیره تنظیمات کلی</button>
				</form>
			</section>
		</main>
	</div>
	{{safeHTML .ModalHTML}}
	{{safeHTML .PanelScript}}
</body>
</html>
`, data)
}

func renderFullChartHTML(chartJSON string) string {
	data := struct {
		ChartRaw      string
		RangeDropdown string
		CoreScript    string
	}{
		ChartRaw:      safeJSON(chartJSON, `{"hourly":{"categories":[],"series":[]},"daily":{"categories":[],"series":[]}}`),
		RangeDropdown: chartRangeDropdownHTML(),
		CoreScript:    chartCoreScript(),
	}
	return renderTemplate("full-chart", `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no, viewport-fit=cover">
	<title>نمودار ترافیک کلاستر</title>
	<script src="https://cdn.tailwindcss.com"></script>
	<script src="https://cdn.jsdelivr.net/npm/apexcharts"></script>
	<link href="https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;600;800;900&display=swap" rel="stylesheet">
	<style>
		html,body{height:100%;}
		body{font-family:'Vazirmatn',sans-serif;background:#020617;margin:0;overflow:hidden;}
		[x-cloak]{display:none !important;}
		.mesh{background:radial-gradient(circle at 20% 0%,rgba(34,211,238,.16),transparent 40%),radial-gradient(circle at 80% 100%,rgba(52,211,153,.12),transparent 45%),#020617;}
		.apexcharts-tooltip{background:#0f172a !important;border:1px solid #334155 !important;color:#f8fafc !important;}
		#fullWrap:fullscreen, #fullWrap:-webkit-full-screen { background:#020617; padding:12px; }
	</style>
	{{safeHTML .CoreScript}}
	<script>window.CHART_RAW = {{safeJS .ChartRaw}};</script>
</head>
<body class="mesh text-slate-100" x-data="svmMakeChartComponent(window.CHART_RAW, 'fullChart')" x-init="mount()">
	<div id="fullWrap" class="h-[100dvh] flex flex-col p-3 sm:p-4">
		<div class="shrink-0 flex items-center justify-between gap-3 mb-3 relative z-30">
			<div class="min-w-0">
				<h1 class="text-base sm:text-lg font-black bg-gradient-to-l from-cyan-300 to-emerald-300 bg-clip-text text-transparent truncate">📊 نمودار مصرف ترافیک کلاستر</h1>
				<p class="text-[11px] text-slate-500 truncate">مجموع ترافیک همهٔ سرورها در بازهٔ انتخابی</p>
			</div>
			<div class="flex items-center gap-2 shrink-0">
				{{safeHTML .RangeDropdown}}
				<button type="button" onclick="svmGoLandscape(document.getElementById('fullWrap'))" aria-label="افقی / تمام صفحه" class="flex items-center justify-center w-9 h-9 rounded-xl bg-slate-900 border border-slate-700 text-slate-300 active:scale-95">
					<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H5a2 2 0 0 0-2 2v3"/><path d="M16 3h3a2 2 0 0 1 2 2v3"/><path d="M8 21H5a2 2 0 0 1-2-2v-3"/><path d="M16 21h3a2 2 0 0 0 2-2v-3"/></svg>
				</button>
			</div>
		</div>
		<div class="flex-1 min-h-0 rounded-2xl border border-white/10 bg-slate-950/40 p-2 relative z-0">
			<div id="fullChart" class="w-full h-full"></div>
		</div>
		<div class="shrink-0 mt-3 flex flex-wrap items-center justify-between gap-2">
			<p class="text-xs sm:text-sm font-black" :class="chartTrendUp === null ? 'text-slate-400' : (chartTrendUp ? 'text-emerald-400' : 'text-rose-400')"><span x-text="chartTrendText"></span></p>
			<p class="text-[11px] sm:text-xs text-slate-400">مصرف در این بازه: <span class="text-slate-100 font-black" x-text="chartTotal + ' GB'"></span></p>
		</div>
	</div>
	<script src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js"></script>
</body>
</html>
`, data)
}

func dashboardModalsHTML() string {
	return `
	<div x-show="editUserModal" x-cloak class="fixed inset-0 z-[70] bg-black/80 backdrop-blur-sm flex items-end sm:items-center justify-center p-0 sm:p-4" x-transition.opacity @click.self="editUserModal=false" @keydown.escape.window="editUserModal=false">
		<div class="w-full max-w-xl max-h-[92vh] overflow-y-auto rounded-t-[2rem] sm:rounded-[2rem] bg-slate-900 border border-white/10 shadow-2xl p-5 sm:p-7">
			<div class="flex items-center justify-between mb-5">
				<h3 class="text-xl font-black text-cyan-300" x-text="selectedUser.mode === 'create' ? '➕ کاربر جدید' : (selectedUser.mode === 'node' ? '🖥️ ویرایش نود' : '✏️ ویرایش کاربر')"></h3>
				<button @click="editUserModal=false" class="w-10 h-10 rounded-2xl bg-slate-800 hover:bg-slate-700 text-slate-300 text-2xl leading-none">×</button>
			</div>
			<div x-show="selectedUser.mode === 'create'">
				<form action="/admin/actions" method="POST" class="space-y-4">
					<input type="hidden" name="action" value="create_user">
					<input type="hidden" name="current_tab" value="users">
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">نام کاربری</label>
						<div class="flex gap-2">
							<input type="text" x-model="newUsername" name="username" required dir="ltr" class="flex-1 bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none text-left focus:border-cyan-400">
							<button type="button" @click="newUsername=randomUsername()" class="rounded-2xl bg-cyan-500/10 text-cyan-300 px-4 font-black">🎲</button>
						</div>
					</div>
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">رمز عبور</label>
						<div class="flex gap-2">
							<input type="text" x-model="newPassword" name="password" required dir="ltr" class="flex-1 bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none text-left focus:border-cyan-400">
							<button type="button" @click="newPassword=randomPassword()" class="rounded-2xl bg-cyan-500/10 text-cyan-300 px-4 font-black">🎲</button>
						</div>
					</div>
					<div class="grid grid-cols-3 gap-3">
						<div><label class="block text-xs font-bold text-slate-400 mb-2">اعتبار روز</label><input type="number" name="days" required class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-xs font-bold text-slate-400 mb-2">حجم GB</label><input type="number" step="any" name="volume" required class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-xs font-bold text-slate-400 mb-2">UDPGW</label><input type="number" name="udpgw" value="7301" required class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
					</div>
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">یادداشت</label>
						<textarea name="comment" rows="3" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></textarea>
					</div>
					<button type="submit" class="w-full rounded-2xl bg-gradient-to-l from-cyan-500 to-emerald-500 text-slate-950 py-3.5 font-black">ایجاد کاربر</button>
				</form>
			</div>
			<div x-show="selectedUser.mode === 'full_edit'">
				<form action="/admin/actions" method="POST" class="space-y-4">
					<input type="hidden" name="action" value="full_edit_user">
					<input type="hidden" name="current_tab" value="users">
					<input type="hidden" name="username" :value="selectedUser.username">
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">نام کاربری</label>
						<input type="text" :value="selectedUser.username" disabled dir="ltr" class="w-full bg-slate-950/40 border border-slate-800 rounded-2xl px-4 py-3 text-slate-500 font-bold text-left">
					</div>
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">رمز عبور</label>
						<div class="flex gap-2">
							<input type="text" x-model="newPassword" name="password" required dir="ltr" class="flex-1 bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none text-left focus:border-indigo-400">
							<button type="button" @click="newPassword=randomPassword()" class="rounded-2xl bg-indigo-500/10 text-indigo-300 px-4 font-black">🎲</button>
						</div>
					</div>
					<div class="grid grid-cols-3 gap-3">
						<div><label class="block text-xs font-bold text-slate-400 mb-2">اعتبار از الان</label><input type="number" name="days" x-model="editDays" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-xs font-bold text-slate-400 mb-2">حجم کل GB</label><input type="number" step="any" name="volume" x-model="editVol" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
						<div><label class="block text-xs font-bold text-slate-400 mb-2">UDPGW</label><input type="number" name="udpgw" x-model="editUdpgw" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></div>
					</div>
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">یادداشت</label>
						<textarea name="comment" x-model="editComment" rows="3" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none"></textarea>
					</div>
					<button type="submit" class="w-full rounded-2xl bg-indigo-500 text-white py-3.5 font-black">ذخیره ویرایش</button>
				</form>
			</div>
			<div x-show="selectedUser.mode === 'node'">
				<form action="/admin/actions" method="POST" class="space-y-4">
					<input type="hidden" name="action" value="edit_node">
					<input type="hidden" name="current_tab" value="nodes">
					<input type="hidden" name="ip" :value="selectedUser.IP">
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">آی‌پی</label>
						<input type="text" :value="selectedUser.IP" disabled dir="ltr" class="w-full bg-slate-950/40 border border-slate-800 rounded-2xl px-4 py-3 text-slate-500 font-mono text-left">
					</div>
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">دامنه اتصال</label>
						<input type="text" name="domain" x-model="selectedUser.Domain" placeholder="node.example.com" dir="ltr" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none text-left font-mono">
					</div>
					<div>
						<label class="block text-xs font-bold text-slate-400 mb-2">نام نمایشی</label>
						<input type="text" name="remark" x-model="selectedUser.CustomRemark" placeholder="🇩🇪 Germany" class="w-full bg-slate-950/70 border border-slate-700 rounded-2xl px-4 py-3 outline-none">
					</div>
					<button type="submit" class="w-full rounded-2xl bg-emerald-500 text-slate-950 py-3.5 font-black">ثبت تغییرات نود</button>
				</form>
			</div>
		</div>
	</div>
	<div x-show="drilldownModal" x-cloak class="fixed inset-0 z-[80] bg-black/80 backdrop-blur-sm flex items-center justify-center p-3 sm:p-5" x-transition.opacity @click.self="drilldownModal=false; drilldownUrl=''">
		<div class="w-full max-w-6xl h-[86vh] rounded-[2rem] bg-slate-900 border border-white/10 shadow-2xl p-4 flex flex-col">
			<div class="flex items-center justify-between mb-4 shrink-0">
				<h3 class="text-xl font-black text-cyan-300">📊 جزئیات نمودار</h3>
				<button @click="drilldownModal=false; drilldownUrl=''" class="w-10 h-10 rounded-2xl bg-slate-800 hover:bg-slate-700 text-slate-300 text-2xl leading-none">×</button>
			</div>
			<div class="flex-1 min-h-0 rounded-2xl overflow-hidden bg-slate-950 border border-slate-800">
				<template x-if="drilldownModal">
					<iframe :src="drilldownUrl" class="w-full h-full border-0 block"></iframe>
				</template>
			</div>
		</div>
	</div>
`
}

func chartCoreScript() string {
	return `
<script>
	var CHART_RANGES = {
		"1h":  { t: "h", n: 1 },  "2h":  { t: "h", n: 2 },  "4h":  { t: "h", n: 4 },
		"6h":  { t: "h", n: 6 },  "12h": { t: "h", n: 12 }, "24h": { t: "h", n: 24 },
		"2d":  { t: "d", n: 2 },  "3d":  { t: "d", n: 3 },  "5d":  { t: "d", n: 5 },
		"7d":  { t: "d", n: 7 },  "14d": { t: "d", n: 14 }, "30d": { t: "d", n: 30 },
		"90d": { t: "d", n: 90 }, "all": { t: "d", n: 0 }
	};
	var CHART_RANGE_OPTIONS = [
		{ v: "1h", l: "۱ ساعت" }, { v: "2h", l: "۲ ساعت" }, { v: "4h", l: "۴ ساعت" },
		{ v: "6h", l: "۶ ساعت" }, { v: "12h", l: "۱۲ ساعت" }, { v: "24h", l: "۲۴ ساعت" },
		{ v: "2d", l: "۲ روز" }, { v: "3d", l: "۳ روز" }, { v: "5d", l: "۵ روز" },
		{ v: "7d", l: "۷ روز" }, { v: "14d", l: "۱۴ روز" }, { v: "30d", l: "۳۰ روز" },
		{ v: "90d", l: "۳ ماه" }, { v: "all", l: "همه" }
	];

	function svmBuildChart(raw, rangeKey) {
		var meta = CHART_RANGES[rangeKey] || CHART_RANGES["7d"];
		raw = raw || { hourly: { categories: [], series: [] }, daily: { categories: [], series: [] } };
		var part = (meta.t === "h") ? (raw.hourly || { categories: [], series: [] }) : (raw.daily || { categories: [], series: [] });
		var cats = (part.categories || []).slice();
		var series = (part.series || []).map(function(s) { return { name: s.name, data: (s.data || []).slice() }; });
		if (meta.n > 0) {
			cats = cats.slice(-meta.n);
			series = series.map(function(s) { s.data = s.data.slice(-meta.n); return s; });
		}
		var len = cats.length, totals = [], i;
		for (i = 0; i < len; i++) totals[i] = 0;
		series.forEach(function(s) { for (var j = 0; j < len; j++) totals[j] += Number(s.data[j] || 0); });
		var total = 0;
		for (i = 0; i < len; i++) total += totals[i];
		var half = Math.floor(len / 2), trendUp = null, trendText = "روند: دادهٔ ناکافی";
		if (len >= 2 && half >= 1) {
			var first = 0, second = 0;
			for (var a = 0; a < half; a++) first += totals[a];
			for (var b = half; b < len; b++) second += totals[b];
			var fa = first / half, sa = second / (len - half);
			if (fa <= 0.0001) {
				trendUp = sa > 0 ? true : null;
				trendText = sa > 0 ? "روند: افزایشی (بدون پایهٔ قبلی)" : "روند: بدون تغییر";
			} else {
				var pct = ((sa - fa) / fa) * 100;
				if (Math.abs(pct) < 0.5) { trendUp = null; trendText = "روند: تقریباً ثابت"; }
				else if (pct > 0) { trendUp = true; trendText = "روند: افزایش " + pct.toFixed(1) + "٪ ↗"; }
				else { trendUp = false; trendText = "روند: کاهش " + Math.abs(pct).toFixed(1) + "٪ ↘"; }
			}
		}
		return { categories: cats, series: series, total: total.toFixed(2), trendUp: trendUp, trendText: trendText };
	}

	function svmChartOptions(built) {
		return {
			series: built.series,
			chart: { type: "bar", height: "100%", stacked: true, toolbar: { show: false }, fontFamily: "Vazirmatn, Tahoma, sans-serif", foreColor: "#94A3B8", background: "transparent" },
			plotOptions: { bar: { horizontal: false, borderRadius: 6, columnWidth: "55%" } },
			dataLabels: { enabled: false },
			xaxis: { categories: built.categories, axisBorder: { show: false }, axisTicks: { show: false }, labels: { rotate: -45, rotateAlways: built.categories.length > 14, style: { fontSize: "10px" } } },
			yaxis: { labels: { formatter: function(val) { if (!val) return "0 B"; if (val < 1) return (val * 1024).toFixed(0) + " MB"; return Math.round(val) + " GB"; } } },
			grid: { borderColor: "#334155", strokeDashArray: 5 },
			theme: { mode: "dark" },
			colors: ["#60A5FA", "#34D399", "#FBBF24", "#F87171", "#A78BFA", "#F472B6", "#38BDF8"],
			tooltip: { theme: "dark", y: { formatter: function(val) { return (val || 0).toFixed(3) + " GB"; } } },
			legend: { position: "top", horizontalAlign: "center", labels: { colors: "#E2E8F0" } },
			noData: { text: "در این بازه ترافیکی ثبت نشده است — بازهٔ بزرگ‌تر را انتخاب کنید", align: "center", verticalAlign: "middle", style: { color: "#94A3B8", fontSize: "13px", fontFamily: "Vazirmatn, Tahoma, sans-serif" } }
		};
	}

	function svmAttachResize(chart) {
		var fn = function() { try { chart.resize(); } catch (e) {} };
		window.addEventListener("resize", fn);
		window.addEventListener("orientationchange", function() { setTimeout(fn, 250); });
		document.addEventListener("fullscreenchange", function() { setTimeout(fn, 250); });
		document.addEventListener("webkitfullscreenchange", function() { setTimeout(fn, 250); });
	}

	// Toggle real fullscreen + force landscape (the only reliable way on mobile).
	function svmGoLandscape(el) {
		if (document.fullscreenElement || document.webkitFullscreenElement) {
			if (document.exitFullscreen) { document.exitFullscreen(); return; }
			if (document.webkitExitFullscreen) { document.webkitExitFullscreen(); return; }
		}
		el = el || document.documentElement;
		function lock() { try { if (screen.orientation && screen.orientation.lock) { screen.orientation.lock("landscape").catch(function(){}); } } catch (e) {} }
		if (el.requestFullscreen) { el.requestFullscreen().then(lock).catch(function(){}); }
		else if (el.webkitRequestFullscreen) { el.webkitRequestFullscreen(); setTimeout(lock, 250); }
	}

	// Self-contained range dropdown. Owns its own scope; broadcasts the chosen
	// range via "svm-range-change" so any chart component can react.
	function svmRangeDropdown() {
		return {
			open: false,
			range: "7d",
			options: (typeof CHART_RANGE_OPTIONS !== "undefined") ? CHART_RANGE_OPTIONS : [],
			label: function() {
				var o = (this.options || []).find(function(x){ return x.v === this.range; }.bind(this));
				return o ? o.l : this.range;
			},
			pick: function(v) {
				this.range = v;
				this.open = false;
				window.dispatchEvent(new CustomEvent("svm-range-change", { detail: v }));
			}
		};
	}

	// Reusable Alpine chart component. The mount method is NOT named "init"
	// (Alpine 3 auto-calls a method named init, which caused a double render);
	// it is invoked explicitly via x-init="mount()".
	function svmMakeChartComponent(raw, elId) {
		return {
			chartRaw: raw || { hourly: { categories: [], series: [] }, daily: { categories: [], series: [] } },
			chartRange: "7d",
			chartTotal: "0.00",
			chartTrendUp: null,
			chartTrendText: "روند: —",
			chart: null,
			buildAndApply: function() {
				var r = svmBuildChart(this.chartRaw, this.chartRange);
				this.chartTotal = r.total;
				this.chartTrendUp = r.trendUp;
				this.chartTrendText = r.trendText;
				var built = { categories: r.categories, series: r.series };
				if (this.chart) { this.chart.updateOptions(svmChartOptions(built), true, true); }
				return built;
			},
			mount: function() {
				var self = this;
				var id = elId;
				window.addEventListener("svm-range-change", function(e){ self.chartRange = e.detail; self.buildAndApply(); });
				this.$nextTick(function() {
					setTimeout(function() {
						var el = document.getElementById(id);
						if (!el || !window.ApexCharts || self.chart) return;
						var built = self.buildAndApply();
						self.chart = new ApexCharts(el, svmChartOptions(built));
						self.chart.render();
						svmAttachResize(self.chart);
					}, 120);
				});
			}
		};
	}
</script>
`
}

func panelDataScript() string {
	return chartCoreScript() + `
<script>
	function flashMessage(message) {
		window.dispatchEvent(new CustomEvent("toast", { detail: message || "انجام شد" }));
	}
	function copyText(text) {
		if (!text) return;
		function fallbackCopy(value) {
			var ta = document.createElement("textarea");
			ta.value = value;
			ta.style.position = "fixed";
			ta.style.opacity = "0";
			document.body.appendChild(ta);
			ta.select();
			try { document.execCommand("copy"); flashMessage("کپی شد"); }
			catch (e) { alert("امکان کپی خودکار وجود ندارد."); }
			ta.remove();
		}
		if (navigator.clipboard && window.isSecureContext) {
			navigator.clipboard.writeText(text).then(function(){ flashMessage("کپی شد"); }).catch(function(){ fallbackCopy(text); });
		} else {
			fallbackCopy(text);
		}
	}
	function copySubLink(token) { copyText(window.location.origin + "/sub/" + token); }
	function readStoredPerPage() {
		try {
			var v = parseInt(localStorage.getItem("svm_perpage") || "10", 10);
			if (v === 20 || v === 50 || v === 100) return v;
		} catch (e) {}
		return 10;
	}
	function panelData() {
		return {
			activeTab: window.SERVER_DATA.currentTab || "dashboard",
			editUserModal: false,
			selectedUser: {},
			drilldownModal: false,
			drilldownUrl: "",
			newUsername: "",
			newPassword: "",
			editDays: 30,
			editVol: 0,
			editUdpgw: 7301,
			editComment: "",
			onlineMap: window.SERVER_DATA.onlineMap || {},
			allUsers: window.SERVER_DATA.allUsers || [],
			nodes: window.SERVER_DATA.nodes || [],
			cpu: window.SERVER_DATA.cpu || 0,
			ram: window.SERVER_DATA.ram || 0,
			stats: Object.assign({ totalUsers: 0, onlineUsers: 0, inactiveUsers: 0, totalNodes: 0, activeNodes: 0 }, window.SERVER_DATA.stats || {}),
			toastMsg: window.SERVER_DATA.toastMsg || "",
			toastIsError: false,
			showToast: false,
			toastTimer: null,
			refreshInterval: Number(localStorage.getItem("svm_refresh") || 20),
			timer: null,
			userSearch: "",
			userFilter: "all",
			currentPage: 1,
			itemsPerPage: readStoredPerPage(),
			sortCol: "username",
			sortAsc: true,
			chartRendered: false,
			chart: null,
			chartRange: "7d",
			chartTotal: "0.00",
			chartTrendText: "روند: —",
			chartTrendUp: null,
			chartRaw: { hourly: { categories: [], series: [] }, daily: { categories: [], series: [] } },

			normalizeChartRaw: function(raw) {
				var emptyPart = { categories: [], series: [] };
				if (raw && typeof raw === "object" && !Array.isArray(raw)) {
					return {
						hourly: (raw.hourly && Array.isArray(raw.hourly.categories)) ? raw.hourly : emptyPart,
						daily:  (raw.daily  && Array.isArray(raw.daily.categories))  ? raw.daily  : emptyPart
					};
				}
				return { hourly: emptyPart, daily: emptyPart };
			},

			openFullChart: function() {
				if (document.fullscreenElement || document.webkitFullscreenElement) {
					if (document.exitFullscreen) { document.exitFullscreen(); return; }
					if (document.webkitExitFullscreen) { document.webkitExitFullscreen(); return; }
				}
				var el = document.getElementById("dashChartWrap");
				function lock() { try { if (screen.orientation && screen.orientation.lock) { screen.orientation.lock("landscape").catch(function(){}); } } catch (e) {} }
				if (el && el.requestFullscreen) {
					el.requestFullscreen().then(lock).catch(function(){ window.open("/admin/chart-full", "_blank"); });
				} else if (el && el.webkitRequestFullscreen) {
					el.webkitRequestFullscreen(); setTimeout(lock, 250);
				} else {
					window.open("/admin/chart-full", "_blank");
				}
			},

			buildChartData: function() {
				var r = svmBuildChart(this.chartRaw, this.chartRange);
				this.chartTotal = r.total;
				this.chartTrendUp = r.trendUp;
				this.chartTrendText = r.trendText;
				return { categories: r.categories, series: r.series };
			},
			chartOptions: function(built) { return svmChartOptions(built); },
			onChartRangeChange: function() {
				var built = this.buildChartData();
				if (this.chart) { this.chart.updateOptions(this.chartOptions(built), true, true); }
			},

			get sortedUsers() {
				var self = this;
				var list = Array.isArray(this.allUsers) ? this.allUsers.slice() : [];
				if (this.userFilter === "online") {
					list = list.filter(function(u) { return self.isActive(u) && self.onlineMap && self.onlineMap[u.username]; });
				} else if (this.userFilter === "inactive") {
					list = list.filter(function(u) { return !self.isActive(u); });
				}
				var q = (this.userSearch || "").trim().toLowerCase();
				if (q !== "") {
					list = list.filter(function(u) {
						return String(u.username || "").toLowerCase().indexOf(q) !== -1 ||
							String(u.comment || "").toLowerCase().indexOf(q) !== -1;
					});
				}
				var dir = this.sortAsc ? 1 : -1;
				var col = this.sortCol;
				list.sort(function(a, b) {
					var av = self.sortValue(a, col);
					var bv = self.sortValue(b, col);
					if (typeof av === "string" || typeof bv === "string") {
						return String(av).localeCompare(String(bv)) * dir;
					}
					return ((av || 0) - (bv || 0)) * dir;
				});
				return list;
			},
			get totalPages() { return Math.max(1, Math.ceil((this.sortedUsers || []).length / this.itemsPerPage)); },
			get paginatedUsers() {
				if (this.currentPage > this.totalPages) this.currentPage = this.totalPages;
				var start = (this.currentPage - 1) * this.itemsPerPage;
				return (this.sortedUsers || []).slice(start, start + this.itemsPerPage);
			},
			notify: function(message, isError) {
				var self = this;
				this.toastMsg = message || "انجام شد";
				this.toastIsError = !!isError;
				this.showToast = true;
				clearTimeout(this.toastTimer);
				this.toastTimer = setTimeout(function() { self.showToast = false; }, 2800);
			},
			onPerPageChange: function() {
				this.currentPage = 1;
				try { localStorage.setItem("svm_perpage", String(this.itemsPerPage)); } catch (e) {}
			},
			cleanToastFromURL: function() {
				try {
					var p = new URLSearchParams(window.location.search);
					var tab = p.get("tab");
					var np = new URLSearchParams();
					if (tab) np.set("tab", tab);
					var s = np.toString();
					window.history.replaceState(null, "", window.location.pathname + (s ? "?" + s : ""));
				} catch (e) {}
			},
			sortValue: function(user, col) {
				if (col === "username") return String(user.username || "").toLowerCase();
				if (col === "data_used") return Number(user.data_used || 0);
				if (col === "expiry") return Number(user.expiry_unix || 0);
				if (col === "last_seen") return Number(user.last_seen || 0);
				if (col === "status") return this.isActive(user) ? (this.onlineMap[user.username] ? 2 : 1) : 0;
				return String(user.username || "").toLowerCase();
			},
			sortBy: function(col) {
				if (this.sortCol === col) this.sortAsc = !this.sortAsc;
				else { this.sortCol = col; this.sortAsc = col === "username"; }
				this.currentPage = 1;
			},
			sortIcon: function(col) {
				if (this.sortCol !== col) return "";
				return this.sortAsc ? "↑" : "↓";
			},
			isActive: function(user) {
				if (!user) return false;
				var notExpired = Number(user.expiry_unix || 0) * 1000 > Date.now();
				var hasVolume = Number(user.data_limit || 0) === 0 || Number(user.data_used || 0) < Number(user.data_limit || 0);
				return notExpired && hasVolume;
			},
			daysLeft: function(user) {
				if (!user) return 0;
				return Math.ceil(((Number(user.expiry_unix || 0) * 1000) - Date.now()) / 86400000);
			},
			usagePercent: function(user) {
				if (!user || Number(user.data_limit || 0) === 0) return 0;
				var p = (Number(user.data_used || 0) / Number(user.data_limit || 1)) * 100;
				return Math.max(0, Math.min(100, p));
			},
			formatBytes: function(bytes) {
				bytes = Number(bytes || 0);
				var gb = 1024 * 1024 * 1024;
				var mb = 1024 * 1024;
				if (bytes >= gb) return (bytes / gb).toFixed(2) + " GB";
				return (bytes / mb).toFixed(1) + " MB";
			},
			formatGB: function(bytes) { return (Number(bytes || 0) / (1024 * 1024 * 1024)).toFixed(2) + " GB"; },
			limitText: function(user) {
				if (!user || Number(user.data_limit || 0) === 0) return "نامحدود";
				return (Number(user.data_limit || 0) / (1024 * 1024 * 1024)).toFixed(2) + " GB";
			},
			dateFa: function(unix, withTime) {
				unix = Number(unix || 0);
				if (unix <= 0) return "ندارد";
				var d = new Date(unix * 1000);
				return withTime ? d.toLocaleString("fa-IR") : d.toLocaleDateString("fa-IR");
			},
			randomUsername: function() { return "user_" + Math.floor(10000 + Math.random() * 90000); },
			randomPassword: function() {
				var chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
				var p = "";
				for (var i = 0; i < 10; i++) p += chars.charAt(Math.floor(Math.random() * chars.length));
				return p;
			},
			prepareCreateUser: function() {
				this.selectedUser = { mode: "create" };
				this.newUsername = "";
				this.newPassword = "";
				this.editUserModal = true;
			},
			prepareEditUser: function(user) {
				var u = Object.assign({}, user || {});
				u.mode = "full_edit";
				this.selectedUser = u;
				this.newPassword = u.password || "";
				var d = this.daysLeft(u);
				this.editDays = d > 0 ? d : 0;
				this.editVol = (Number(u.data_limit || 0) / (1024 * 1024 * 1024)).toFixed(2);
				this.editUdpgw = u.udpgw || 7301;
				this.editComment = u.comment || "";
				this.editUserModal = true;
			},
			prepareNodeEdit: function(node) {
				var n = Object.assign({}, node || {});
				n.mode = "node";
				this.selectedUser = n;
				this.editUserModal = true;
			},
			fetchLiveData: function() {
				var self = this;
				fetch("/admin/api/live-data", { cache: "no-store" })
					.then(function(res) {
						if (!res.ok) throw new Error("live data failed");
						return res.json();
					})
					.then(function(data) {
						self.cpu = Number(data.cpu || 0);
						self.ram = Number(data.ram || 0);
						self.allUsers = data.users || [];
						self.nodes = data.nodes || [];
						self.onlineMap = data.onlineMap || {};
						self.stats = Object.assign({ totalUsers: 0, onlineUsers: 0, inactiveUsers: 0, totalNodes: 0, activeNodes: 0 }, data.stats || {});
						if (self.currentPage > self.totalPages) self.currentPage = self.totalPages;
					})
					.catch(function(err) { console.error(err); });
			},
			renderDashboardChart: function() {
				var self = this;
				if (this.chartRendered) return;
				this.$nextTick(function() {
					setTimeout(function() {
						var el = document.querySelector("#dashboardChart");
						if (!el || !window.ApexCharts || self.chartRendered) return;
						var built = self.buildChartData();
						self.chart = new ApexCharts(el, self.chartOptions(built));
						self.chart.render();
						svmAttachResize(self.chart);
						self.chartRendered = true;
					}, 120);
				});
			},
			initSetup: function() {
				var self = this;
				this.chartRaw = this.normalizeChartRaw(window.SERVER_DATA.chartData);
				window.addEventListener("toast", function(e) { self.notify(e.detail || "انجام شد", false); });
				window.addEventListener("svm-range-change", function(e){ self.chartRange = e.detail; self.onChartRangeChange(); });
				this.fetchLiveData();
				if (this.refreshInterval > 0) {
					this.timer = setInterval(function() { self.fetchLiveData(); }, this.refreshInterval * 1000);
				}
				if (this.toastMsg !== "") {
					setTimeout(function() {
						self.notify(self.toastMsg, !!window.SERVER_DATA.initialToastIsError);
						self.cleanToastFromURL();
					}, 150);
				}
				this.$watch("activeTab", function(val) {
					if (val === "dashboard") self.renderDashboardChart();
					var desired = "?tab=" + encodeURIComponent(val);
					if (window.location.search !== desired) {
						window.history.pushState(null, "", desired);
					}
				});
				if (this.activeTab === "dashboard") {
					this.renderDashboardChart();
				}
			}
		};
	}
</script>
`
}
