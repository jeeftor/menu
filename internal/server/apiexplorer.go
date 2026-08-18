package server

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"menu/internal/nutrislice"
)

func (s *Server) handleAPIExplorer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	now := time.Now()
	schoolOpts := buildExplorerSchoolOptions()
	schoolOptsAll := `<option value="">All schools</option>` + schoolOpts
	mealOpts := `<option value="lunch">Lunch</option><option value="breakfast">Breakfast</option>`

	repl := strings.NewReplacer(
		"[[VERSION]]", s.version,
		"[[YEAR]]", strconv.Itoa(now.Year()),
		"[[MONTH]]", strconv.Itoa(int(now.Month())),
		"[[SCHOOL_OPTS]]", schoolOpts,
		"[[SCHOOL_OPTS_ALL]]", schoolOptsAll,
		"[[MEAL_OPTS]]", mealOpts,
	)
	fmt.Fprint(w, repl.Replace(apiExplorerPage))
}

func buildExplorerSchoolOptions() string {
	var sb strings.Builder
	for _, s := range nutrislice.DefaultSchools {
		sb.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`,
			html.EscapeString(s.Slug), html.EscapeString(s.Name)))
	}
	return sb.String()
}

const apiExplorerPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Menu API Explorer</title>
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    html,body{height:100%}
    body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0F172A;color:#E2E8F0;display:flex;flex-direction:column;height:100vh;overflow:hidden}
    /* Header */
    header{background:#020617;padding:.65rem 1.25rem;display:flex;align-items:center;gap:.75rem;border-bottom:1px solid #1E293B;flex-shrink:0}
    .hdr-logo{font-weight:700;font-size:1rem;white-space:nowrap}
    .hdr-ver{font-size:.68rem;color:#475569;font-weight:400;margin-left:.3rem}
    .hdr-nav{display:flex;gap:.5rem;margin-left:auto}
    .hdr-link{color:#64748B;text-decoration:none;font-size:.78rem;padding:.28rem .6rem;border:1px solid #334155;border-radius:6px;transition:all .15s}
    .hdr-link:hover{color:#E2E8F0;border-color:#475569}
    .tabs{display:flex;gap:.3rem;background:#1E293B;padding:.2rem;border-radius:8px;margin-left:.75rem}
    .tab-btn{padding:.28rem .85rem;border-radius:6px;border:none;cursor:pointer;font-size:.8rem;font-weight:600;background:transparent;color:#64748B;transition:all .15s}
    .tab-btn.active{background:#3B82F6;color:#fff}
    /* Layout */
    .layout{display:flex;flex:1;overflow:hidden}
    .sidebar{width:210px;background:#0B1120;padding:.9rem;overflow-y:auto;flex-shrink:0;border-right:1px solid #1E293B}
    .main{flex:1;overflow-y:auto;padding:1.25rem 1.5rem}
    /* Sidebar */
    .sg-title{font-size:.6rem;font-weight:800;text-transform:uppercase;letter-spacing:.08em;color:#475569;margin:1rem 0 .35rem;padding-left:.4rem}
    .sg-title:first-child{margin-top:0}
    .sl{display:block;color:#64748B;font-size:.78rem;padding:.26rem .55rem;border-radius:6px;text-decoration:none;transition:all .15s}
    .sl:hover{background:#1E293B;color:#CBD5E1}
    /* Endpoint card */
    .ep{background:#1E293B;border-radius:10px;margin-bottom:1.25rem;overflow:hidden;border:1px solid #334155}
    .ep-hdr{padding:.8rem 1.1rem;display:flex;align-items:center;gap:.65rem}
    .m{font-size:.62rem;font-weight:900;padding:.18rem .45rem;border-radius:4px;letter-spacing:.06em;flex-shrink:0;font-family:"SF Mono",Consolas,monospace}
    .GET{background:#059669;color:#fff}
    .POST{background:#2563EB;color:#fff}
    .DELETE{background:#DC2626;color:#fff}
    .ep-path{font-family:"SF Mono",Consolas,monospace;font-size:.85rem;color:#93C5FD;font-weight:600}
    .ep-tag{color:#64748B;font-size:.75rem;margin-left:auto;white-space:nowrap}
    .ep-body{padding:.9rem 1.1rem;border-top:1px solid #334155}
    .ep-desc{color:#94A3B8;font-size:.82rem;margin-bottom:.8rem;line-height:1.55}
    .ep-desc a{color:#93C5FD}
    .ep-desc code{background:#0F172A;padding:.1rem .3rem;border-radius:3px;font-size:.78rem;font-family:"SF Mono",Consolas,monospace}
    /* Params table */
    .pt{width:100%;border-collapse:collapse;font-size:.76rem;margin-bottom:.85rem}
    .pt th{text-align:left;color:#475569;font-size:.62rem;text-transform:uppercase;letter-spacing:.05em;padding:.3rem .45rem;border-bottom:1px solid #334155}
    .pt td{padding:.32rem .45rem;border-bottom:1px solid #1E293B;color:#CBD5E1;vertical-align:top}
    .pt tr:last-child td{border-bottom:none}
    .pn{font-family:"SF Mono",Consolas,monospace;color:#93C5FD}
    .pt td code{background:#0F172A;padding:.08rem .28rem;border-radius:3px;font-size:.72rem;font-family:"SF Mono",Consolas,monospace}
    /* Try It */
    .try-box{background:#0F172A;border-radius:8px;padding:.8rem .9rem;border:1px solid #334155}
    .try-lbl{font-size:.6rem;font-weight:700;text-transform:uppercase;letter-spacing:.06em;color:#475569;margin-bottom:.55rem}
    .try-row{display:flex;flex-wrap:wrap;gap:.45rem;align-items:flex-end}
    .tf{display:flex;flex-direction:column;gap:.18rem}
    .tf label{font-size:.62rem;color:#64748B;font-weight:600;text-transform:uppercase;letter-spacing:.04em}
    .tf input,.tf select{background:#1E293B;border:1px solid #334155;border-radius:5px;color:#E2E8F0;padding:.3rem .55rem;font-size:.78rem;outline:none}
    .tf input:focus,.tf select:focus{border-color:#3B82F6}
    .try-btn{background:#3B82F6;border:none;color:#fff;padding:.3rem .9rem;border-radius:6px;font-size:.78rem;font-weight:700;cursor:pointer;white-space:nowrap;transition:background .15s;align-self:flex-end}
    .try-btn:hover{background:#1D4ED8}
    .copy-btn{background:#1E293B;border:1px solid #334155;color:#94A3B8;padding:.28rem .6rem;border-radius:5px;font-size:.72rem;cursor:pointer;white-space:nowrap;transition:all .15s;align-self:flex-end;font-family:inherit}
    .copy-btn:hover{background:#334155;color:#E2E8F0;border-color:#475569}
    .url-bar{margin-top:.45rem;font-family:"SF Mono",Consolas,monospace;font-size:.68rem;color:#475569;word-break:break-all;display:none}
    .url-bar.show{display:block}
    .resp{margin-top:.55rem;background:#020617;border:1px solid #334155;border-radius:6px;padding:.6rem .8rem;font-family:"SF Mono",Consolas,monospace;font-size:.7rem;color:#86EFAC;white-space:pre-wrap;word-break:break-all;max-height:260px;overflow-y:auto;display:none}
    .resp.show{display:block}
    .resp.err{color:#FCA5A5}
    /* Toast */
    .toast{position:fixed;top:1rem;right:1.25rem;background:#22C55E;color:#fff;font-size:.78rem;font-weight:600;padding:.4rem .9rem;border-radius:8px;box-shadow:0 4px 12px rgba(0,0,0,.45);opacity:0;pointer-events:none;transition:opacity .2s;z-index:9999}
    .toast.show{opacity:1}
    /* Section header */
    .sec-hdr{font-size:.95rem;font-weight:700;color:#F1F5F9;margin-bottom:1rem;padding-bottom:.45rem;border-bottom:1px solid #334155}
    /* MCP view */
    .mcp-wrap{max-width:820px}
    .mcp-card{background:#1E293B;border:1px solid #334155;border-radius:10px;margin-bottom:1.1rem;padding:1.1rem 1.25rem}
    .mcp-tool{font-family:"SF Mono",Consolas,monospace;font-size:.95rem;font-weight:700;color:#93C5FD;margin-bottom:.3rem}
    .mcp-desc{color:#94A3B8;font-size:.82rem;margin-bottom:.7rem;line-height:1.55}
    .code-block{background:#020617;border:1px solid #334155;border-radius:6px;padding:.7rem .9rem;font-family:"SF Mono",Consolas,monospace;font-size:.72rem;color:#BAE6FD;white-space:pre;overflow-x:auto}
    @media(max-width:640px){
      body{overflow:auto;height:auto}
      header{flex-wrap:wrap;padding:.55rem .9rem;gap:.5rem}
      .hdr-logo{font-size:.88rem}
      .tabs{margin-left:0;order:3;width:100%}
      .tab-btn{flex:1;text-align:center}
      .layout{flex-direction:column;overflow:visible}
      .sidebar{display:none}
      .main{overflow:visible;padding:.85rem .9rem}
      .ep-hdr{flex-wrap:wrap;gap:.4rem}
      .ep-tag{margin-left:0;order:3}
      .try-row{flex-direction:column;align-items:stretch}
      .tf input,.tf select{width:100%}
      .try-btn,.copy-btn{width:100%;text-align:center}
    }
  </style>
</head>
<body>
<div class="toast" id="toast"></div>
<header>
  <span class="hdr-logo">&#x1F37D; Menu API Explorer<span class="hdr-ver">v[[VERSION]]</span></span>
  <div class="hdr-nav">
    <a class="hdr-link" href="/">&#x2190; Calendar</a>
    <a class="hdr-link" href="/settings">&#x2699; Settings</a>
  </div>
  <div class="tabs">
    <button class="tab-btn active" onclick="showView('rest')">REST</button>
    <button class="tab-btn" onclick="showView('mcp')">MCP</button>
  </div>
</header>

<!-- REST view -->
<div class="layout" id="view-rest">
  <aside class="sidebar">
    <div class="sg-title">Menu Data</div>
    <a class="sl" href="#ep-schools">Schools</a>
    <a class="sl" href="#ep-lunch">Lunch</a>
    <a class="sl" href="#ep-summary">Summary</a>
    <a class="sl" href="#ep-week">Week</a>
    <a class="sl" href="#ep-month">Month</a>
    <div class="sg-title">Images &amp; Prefs</div>
    <a class="sl" href="#ep-images">Food Images</a>
    <a class="sl" href="#ep-favs">Favorites</a>
    <a class="sl" href="#ep-excl">Exclusions</a>
    <a class="sl" href="#ep-sec">Section Includes</a>
    <div class="sg-title">Utilities</div>
    <a class="sl" href="#ep-missing">Missing Images</a>
  </aside>

  <main id="rest-main">

    <div class="sec-hdr">Menu Data</div>

    <div class="ep" id="ep-schools">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/schools</span>
        <span class="ep-tag">no params</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">Returns the list of ASD20 schools available in this server instance.</p>
        <div class="try-box">
          <div class="try-lbl">Try it</div>
          <div class="try-row">
            <button class="try-btn" onclick="doGet('schools','/api/v1/schools',{})">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/schools',{})" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/schools',{})" title="Copy curl command">$ curl</button>
          </div>
          <div class="url-bar" id="u-schools"></div>
          <pre class="resp" id="r-schools"></pre>
        </div>
      </div>
    </div>

    <div class="ep" id="ep-lunch">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/lunch</span>
        <span class="ep-tag">full day menu</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">Returns the complete menu for a single day — all sections (Option 1–N, Vegetable, Fruit, Milk) and every food item.</p>
        <table class="pt">
          <tr><th>Param</th><th>Default</th><th>Description</th></tr>
          <tr><td class="pn">date</td><td>today</td><td><code>today</code>, <code>next</code>, or <code>YYYY-MM-DD</code></td></tr>
          <tr><td class="pn">school</td><td>woodmen</td><td>School slug or name substring</td></tr>
          <tr><td class="pn">meal</td><td>lunch</td><td><code>lunch</code> or <code>breakfast</code></td></tr>
        </table>
        <div class="try-box">
          <div class="try-lbl">Try it</div>
          <div class="try-row">
            <div class="tf"><label>date</label><input id="p-lunch-date" value="today" style="width:110px"></div>
            <div class="tf"><label>school</label><select id="p-lunch-school">[[SCHOOL_OPTS]]</select></div>
            <div class="tf"><label>meal</label><select id="p-lunch-meal">[[MEAL_OPTS]]</select></div>
            <button class="try-btn" onclick="doGet('lunch','/api/v1/lunch',lunchP())">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/lunch',lunchP())" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/lunch',lunchP())" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-lunch"></div>
          <pre class="resp" id="r-lunch"></pre>
        </div>
      </div>
    </div>

    <div class="ep" id="ep-summary">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/lunch/summary</span>
        <span class="ep-tag">voice-friendly</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">Concise entrée summary. The <code>text</code> field is natural English ready for Alexa/Home Assistant. <code>date=next</code> finds the next school day with data (skips weekends and holidays). Respects exclusion patterns and section include rules configured in <a href="/settings">/settings</a>.</p>
        <table class="pt">
          <tr><th>Param</th><th>Default</th><th>Description</th></tr>
          <tr><td class="pn">date</td><td>next</td><td><code>today</code>, <code>next</code>, or <code>YYYY-MM-DD</code></td></tr>
          <tr><td class="pn">school</td><td>woodmen</td><td>School slug or name substring</td></tr>
          <tr><td class="pn">meal</td><td>lunch</td><td><code>lunch</code> or <code>breakfast</code></td></tr>
        </table>
        <div class="try-box">
          <div class="try-lbl">Try it</div>
          <div class="try-row">
            <div class="tf"><label>date</label><input id="p-sum-date" value="next" style="width:110px"></div>
            <div class="tf"><label>school</label><select id="p-sum-school">[[SCHOOL_OPTS]]</select></div>
            <div class="tf"><label>meal</label><select id="p-sum-meal">[[MEAL_OPTS]]</select></div>
            <button class="try-btn" onclick="doGet('sum','/api/v1/lunch/summary',sumP())">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/lunch/summary',sumP())" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/lunch/summary',sumP())" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-sum"></div>
          <pre class="resp" id="r-sum"></pre>
        </div>
      </div>
    </div>

    <div class="ep" id="ep-week">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/lunch/week</span>
        <span class="ep-tag">Mon–Fri week</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">Returns menus for the Mon–Fri week containing the given date. Only school days with menu data are included.</p>
        <table class="pt">
          <tr><th>Param</th><th>Default</th><th>Description</th></tr>
          <tr><td class="pn">date</td><td>today</td><td>Any date in the target week</td></tr>
          <tr><td class="pn">school</td><td>woodmen</td><td>School slug or name substring</td></tr>
          <tr><td class="pn">meal</td><td>lunch</td><td><code>lunch</code> or <code>breakfast</code></td></tr>
        </table>
        <div class="try-box">
          <div class="try-lbl">Try it</div>
          <div class="try-row">
            <div class="tf"><label>date</label><input id="p-wk-date" value="today" style="width:110px"></div>
            <div class="tf"><label>school</label><select id="p-wk-school">[[SCHOOL_OPTS]]</select></div>
            <div class="tf"><label>meal</label><select id="p-wk-meal">[[MEAL_OPTS]]</select></div>
            <button class="try-btn" onclick="doGet('wk','/api/v1/lunch/week',wkP())">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/lunch/week',wkP())" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/lunch/week',wkP())" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-wk"></div>
          <pre class="resp" id="r-wk"></pre>
        </div>
      </div>
    </div>

    <div class="ep" id="ep-month">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/lunch/month</span>
        <span class="ep-tag">full month</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">Returns all school day menus for the given year/month. The calendar view uses this endpoint internally.</p>
        <table class="pt">
          <tr><th>Param</th><th>Default</th><th>Description</th></tr>
          <tr><td class="pn">year</td><td>(current)</td><td>4-digit year</td></tr>
          <tr><td class="pn">month</td><td>(current)</td><td>1–12</td></tr>
          <tr><td class="pn">school</td><td>woodmen</td><td>School slug or name substring</td></tr>
          <tr><td class="pn">meal</td><td>lunch</td><td><code>lunch</code> or <code>breakfast</code></td></tr>
        </table>
        <div class="try-box">
          <div class="try-lbl">Try it</div>
          <div class="try-row">
            <div class="tf"><label>year</label><input id="p-mo-year" value="[[YEAR]]" style="width:72px"></div>
            <div class="tf"><label>month</label><input id="p-mo-month" value="[[MONTH]]" style="width:56px"></div>
            <div class="tf"><label>school</label><select id="p-mo-school">[[SCHOOL_OPTS]]</select></div>
            <div class="tf"><label>meal</label><select id="p-mo-meal">[[MEAL_OPTS]]</select></div>
            <button class="try-btn" onclick="doGet('mo','/api/v1/lunch/month',moP())">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/lunch/month',moP())" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/lunch/month',moP())" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-mo"></div>
          <pre class="resp" id="r-mo"></pre>
        </div>
      </div>
    </div>

    <div class="sec-hdr" style="margin-top:2rem">Images &amp; Preferences</div>

    <div class="ep" id="ep-images">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/food-images</span>
        <span class="ep-tag">custom image overrides</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">GET returns all custom image entries. POST adds/updates one (JSON body: <code>{food_name, image_url}</code>). DELETE removes by <code>?food_name=</code>. Foods not covered by the Nutrislice API can be given images here.</p>
        <div class="try-box">
          <div class="try-lbl">GET — list all</div>
          <div class="try-row">
            <button class="try-btn" onclick="doGet('img','/api/v1/food-images',{})">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/food-images',{})" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/food-images',{})" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-img"></div>
          <pre class="resp" id="r-img"></pre>
        </div>
      </div>
    </div>

    <div class="ep" id="ep-favs">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/favorites</span>
        <span class="ep-tag">starred foods</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">GET returns all favorites (optionally filtered by <code>?school=</code>). POST adds one (JSON: <code>{food_name, school_slug?}</code>). DELETE removes by <code>?id=</code>.</p>
        <div class="try-box">
          <div class="try-lbl">GET — list all</div>
          <div class="try-row">
            <div class="tf"><label>school (optional)</label><select id="p-favs-school">[[SCHOOL_OPTS_ALL]]</select></div>
            <button class="try-btn" onclick="doGet('favs','/api/v1/favorites',favsP())">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/favorites',favsP())" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/favorites',favsP())" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-favs"></div>
          <pre class="resp" id="r-favs"></pre>
        </div>
      </div>
    </div>

    <div class="ep" id="ep-excl">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/exclusions</span>
        <span class="ep-tag">summary filters</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">Case-insensitive substring patterns excluded from <code>/api/v1/lunch/summary</code> and hidden from the week view display. POST: <code>{pattern, school_slug?}</code>. DELETE: <code>?id=</code>. Manage via <a href="/settings">/settings</a>.</p>
        <div class="try-box">
          <div class="try-lbl">GET — list all</div>
          <div class="try-row">
            <button class="try-btn" onclick="doGet('excl','/api/v1/exclusions',{})">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/exclusions',{})" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/exclusions',{})" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-excl"></div>
          <pre class="resp" id="r-excl"></pre>
        </div>
      </div>
    </div>

    <div class="ep" id="ep-sec">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/section-includes</span>
        <span class="ep-tag">section include rules</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">When include rules exist for a school+meal, only the listed option sections are shown. Non-option sections (Fruit, Vegetable, Milk) always pass through. Useful for hiding the Woodmen Roberts breakfast cereal bar (Option 4). POST: <code>{school_slug, meal_type, section_name}</code>. DELETE: <code>?id=</code>.</p>
        <div class="try-box">
          <div class="try-lbl">GET — list all</div>
          <div class="try-row">
            <button class="try-btn" onclick="doGet('sec','/api/v1/section-includes',{})">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/section-includes',{})" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/section-includes',{})" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-sec"></div>
          <pre class="resp" id="r-sec"></pre>
        </div>
      </div>
    </div>

    <div class="sec-hdr" style="margin-top:2rem">Utilities</div>

    <div class="ep" id="ep-missing">
      <div class="ep-hdr">
        <span class="m GET">GET</span>
        <span class="ep-path">/api/v1/missing-images</span>
        <span class="ep-tag">image gap finder</span>
      </div>
      <div class="ep-body">
        <p class="ep-desc">Scans all cached menu JSON files and returns food names that have no API-provided image and no custom override in the store. Use to find items worth adding in <a href="/settings">/settings</a> &rarr; Custom Food Images.</p>
        <div class="try-box">
          <div class="try-lbl">Try it</div>
          <div class="try-row">
            <button class="try-btn" onclick="doGet('missing','/api/v1/missing-images',{})">Send</button>
            <button class="copy-btn" onclick="copyUrl('/api/v1/missing-images',{})" title="Copy URL">&#x1F517; URL</button>
            <button class="copy-btn" onclick="copyCurl('/api/v1/missing-images',{})" title="Copy curl">$ curl</button>
          </div>
          <div class="url-bar" id="u-missing"></div>
          <pre class="resp" id="r-missing"></pre>
        </div>
      </div>
    </div>

    <div style="height:2rem"></div>
  </main>
</div>

<!-- MCP view -->
<div class="layout" id="view-mcp" style="display:none">
  <main style="padding:1.25rem 1.5rem;overflow-y:auto">
    <div class="mcp-wrap">
      <div class="sec-hdr">MCP — Model Context Protocol</div>
      <p style="color:#94A3B8;font-size:.83rem;margin-bottom:1.25rem;line-height:1.6">
        This server exposes ASD20 school lunch data via MCP, letting AI assistants (Claude, Cursor, Copilot) query menus in natural language. Two transports are available.
      </p>

      <div class="mcp-card">
        <div class="mcp-tool">Streamable HTTP &mdash; POST /mcp</div>
        <p class="mcp-desc">Connect any MCP-compatible client to the HTTP endpoint. Works from any network-reachable host.</p>
        <pre class="code-block">{
  "mcpServers": {
    "menu": {
      "url": "http://localhost:8080/mcp"
    }
  }
}</pre>
      </div>

      <div class="mcp-card">
        <div class="mcp-tool">stdio &mdash; menu mcp</div>
        <p class="mcp-desc">Run as a subprocess for stdio-based clients (Claude Desktop, VS Code MCP extension).</p>
        <pre class="code-block">{
  "mcpServers": {
    "menu": {
      "command": "menu",
      "args": ["mcp"]
    }
  }
}</pre>
      </div>

      <div class="sec-hdr" style="margin-top:2rem">Tools</div>

      <div class="mcp-card">
        <div class="mcp-tool">list_schools</div>
        <p class="mcp-desc">Returns the configured ASD20 schools with their slugs and names.</p>
        <p style="color:#64748B;font-size:.78rem;font-style:italic">No parameters.</p>
      </div>

      <div class="mcp-card">
        <div class="mcp-tool">get_lunch</div>
        <p class="mcp-desc">Returns the complete menu for a school and date — all sections and food items.</p>
        <table class="pt" style="margin-bottom:0">
          <tr><th>Param</th><th>Type</th><th>Description</th></tr>
          <tr><td class="pn">school_slug</td><td>string?</td><td>School slug (omit for default)</td></tr>
          <tr><td class="pn">date</td><td>string?</td><td>YYYY-MM-DD (omit for today)</td></tr>
          <tr><td class="pn">meal_type</td><td>string?</td><td><code>lunch</code> or <code>breakfast</code></td></tr>
        </table>
      </div>

      <div class="mcp-card">
        <div class="mcp-tool">get_lunch_summary</div>
        <p class="mcp-desc">Returns a concise entrée summary. The <code>text</code> field is natural English suitable for voice output. <code>date=next</code> automatically finds the next school day.</p>
        <table class="pt" style="margin-bottom:0">
          <tr><th>Param</th><th>Type</th><th>Description</th></tr>
          <tr><td class="pn">school_slug</td><td>string?</td><td>School slug</td></tr>
          <tr><td class="pn">date</td><td>string?</td><td><code>today</code>, <code>next</code>, or <code>YYYY-MM-DD</code></td></tr>
          <tr><td class="pn">meal_type</td><td>string?</td><td><code>lunch</code> or <code>breakfast</code></td></tr>
        </table>
      </div>

      <div class="mcp-card">
        <div class="mcp-tool">get_lunch_week</div>
        <p class="mcp-desc">Returns menus for the full Mon–Fri week containing the given date.</p>
        <table class="pt" style="margin-bottom:0">
          <tr><th>Param</th><th>Type</th><th>Description</th></tr>
          <tr><td class="pn">school_slug</td><td>string?</td><td>School slug</td></tr>
          <tr><td class="pn">date</td><td>string?</td><td>Any date in the target week</td></tr>
          <tr><td class="pn">meal_type</td><td>string?</td><td><code>lunch</code> or <code>breakfast</code></td></tr>
        </table>
      </div>

      <div style="height:2rem"></div>
    </div>
  </main>
</div>

<script>
function gv(id) { return document.getElementById(id).value; }

// Param builders — read form fields fresh each call
function lunchP() { return {date:gv('p-lunch-date'),school:gv('p-lunch-school'),meal:gv('p-lunch-meal')}; }
function sumP()   { return {date:gv('p-sum-date'),school:gv('p-sum-school'),meal:gv('p-sum-meal')}; }
function wkP()    { return {date:gv('p-wk-date'),school:gv('p-wk-school'),meal:gv('p-wk-meal')}; }
function moP()    { return {year:gv('p-mo-year'),month:gv('p-mo-month'),school:gv('p-mo-school'),meal:gv('p-mo-meal')}; }
function favsP()  { return {school:gv('p-favs-school')}; }

function buildUrl(path, params) {
  var parts = [];
  for (var k in params) {
    var v = params[k];
    if (v !== '' && v !== null && v !== undefined) {
      parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(v));
    }
  }
  return path + (parts.length ? '?' + parts.join('&') : '');
}

function showView(name) {
  document.getElementById('view-rest').style.display = name === 'rest' ? 'flex' : 'none';
  document.getElementById('view-mcp').style.display  = name === 'mcp'  ? 'flex' : 'none';
  var btns = document.querySelectorAll('.tab-btn');
  for (var i = 0; i < btns.length; i++) {
    var t = btns[i].textContent.trim().toLowerCase();
    if (t === name) { btns[i].classList.add('active'); }
    else { btns[i].classList.remove('active'); }
  }
}

function doGet(id, path, params) {
  var url = buildUrl(path, params);
  var el = document.getElementById('r-' + id);
  var ub = document.getElementById('u-' + id);
  if (ub) { ub.textContent = window.location.origin + url; ub.className = 'url-bar show'; }
  el.className = 'resp show';
  el.textContent = 'Loading...';
  fetch(url).then(function(r) {
    return r.text().then(function(t) { return {s: r.status, t: t}; });
  }).then(function(d) {
    try { el.textContent = JSON.stringify(JSON.parse(d.t), null, 2); }
    catch(e) { el.textContent = d.t; }
    el.className = d.s >= 400 ? 'resp show err' : 'resp show';
  }).catch(function(e) {
    el.className = 'resp show err';
    el.textContent = 'Error: ' + e.message;
  });
}

function copyUrl(path, params) {
  var url = window.location.origin + buildUrl(path, params);
  if (navigator.clipboard) {
    navigator.clipboard.writeText(url).then(function() { flash('URL copied!'); });
  } else {
    prompt('Copy this URL:', url);
  }
}

function copyCurl(path, params) {
  var url = window.location.origin + buildUrl(path, params);
  var cmd = "curl '" + url + "'";
  if (navigator.clipboard) {
    navigator.clipboard.writeText(cmd).then(function() { flash('curl copied!'); });
  } else {
    prompt('Copy this curl command:', cmd);
  }
}

function flash(msg) {
  var t = document.getElementById('toast');
  t.textContent = msg;
  t.classList.add('show');
  setTimeout(function() { t.classList.remove('show'); }, 1800);
}
</script>
</body>
</html>`
