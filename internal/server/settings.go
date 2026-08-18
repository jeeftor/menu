package server

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"menu/internal/nutrislice"
	"menu/internal/store"
)

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var imgsHTML, favsHTML, exsHTML, secIncHTML string
	if s.store == nil {
		msg := `<p class="warn">Settings require the server to be started with <code>--data-dir</code>.</p>`
		imgsHTML, favsHTML, exsHTML, secIncHTML = msg, msg, msg, msg
	} else {
		imgs, _ := s.store.ListFoodImages()
		favs, _ := s.store.ListFavorites("")
		exs, _ := s.store.ListExclusions()
		sis, _ := s.store.ListSectionIncludes()
		imgsHTML = buildImagesTable(imgs)
		favsHTML = buildFavoritesTable(favs)
		exsHTML = buildExclusionsTable(exs)
		secIncHTML = buildSectionIncludesTable(sis)
	}

	repl := strings.NewReplacer(
		"[[IMAGES_TABLE]]", imgsHTML,
		"[[FAVORITES_TABLE]]", favsHTML,
		"[[EXCLUSIONS_TABLE]]", exsHTML,
		"[[SECTION_INCLUDES_TABLE]]", secIncHTML,
		"[[SCHOOL_OPTIONS]]", buildSchoolOptions(),
	)
	fmt.Fprint(w, repl.Replace(settingsPage))
}

func buildImagesTable(imgs []store.FoodImage) string {
	if len(imgs) == 0 {
		return `<p class="empty">No custom images yet.</p>`
	}
	var sb strings.Builder
	sb.WriteString(`<table class="data-table"><thead><tr><th>Food Name</th><th>Preview</th><th>URL</th><th></th></tr></thead><tbody>`)
	for _, img := range imgs {
		sb.WriteString(fmt.Sprintf(
			`<tr><td>%s</td>`+
				`<td><img src="%s" class="prev-img" loading="lazy" onerror="this.style.display='none'"></td>`+
				`<td class="url-cell"><a href="%s" target="_blank" rel="noopener">%s</a></td>`+
				`<td><button class="del-btn" data-name="%s" onclick="delImage(this)">Delete</button></td></tr>`,
			html.EscapeString(img.FoodName),
			html.EscapeString(img.ImageURL),
			html.EscapeString(img.ImageURL),
			html.EscapeString(truncURL(img.ImageURL)),
			html.EscapeString(img.FoodName),
		))
	}
	sb.WriteString(`</tbody></table>`)
	return sb.String()
}

func buildFavoritesTable(favs []store.Favorite) string {
	if len(favs) == 0 {
		return `<p class="empty">No favorites yet.</p>`
	}
	var sb strings.Builder
	sb.WriteString(`<table class="data-table"><thead><tr><th>Food Name</th><th>School</th><th></th></tr></thead><tbody>`)
	for _, f := range favs {
		school := f.SchoolSlug
		if school == "" {
			school = "All schools"
		}
		sb.WriteString(fmt.Sprintf(
			`<tr><td>%s</td><td>%s</td>`+
				`<td><button class="del-btn" data-id="%d" onclick="delFav(this)">Delete</button></td></tr>`,
			html.EscapeString(f.FoodName),
			html.EscapeString(school),
			f.ID,
		))
	}
	sb.WriteString(`</tbody></table>`)
	return sb.String()
}

func buildExclusionsTable(exs []store.Exclusion) string {
	if len(exs) == 0 {
		return `<p class="empty">No exclusions configured.</p>`
	}
	var sb strings.Builder
	sb.WriteString(`<table class="data-table"><thead><tr><th>Pattern</th><th>School</th><th></th></tr></thead><tbody>`)
	for _, e := range exs {
		school := e.SchoolSlug
		if school == "" {
			school = "All schools"
		}
		sb.WriteString(fmt.Sprintf(
			`<tr><td><code>%s</code></td><td>%s</td>`+
				`<td><button class="del-btn" data-id="%d" onclick="delExclusion(this)">Delete</button></td></tr>`,
			html.EscapeString(e.Pattern),
			html.EscapeString(school),
			e.ID,
		))
	}
	sb.WriteString(`</tbody></table>`)
	return sb.String()
}

func buildSectionIncludesTable(sis []store.SectionInclude) string {
	if len(sis) == 0 {
		return `<p class="empty">No section include rules. All option sections are shown.</p>`
	}
	var sb strings.Builder
	sb.WriteString(`<table class="data-table"><thead><tr><th>Section</th><th>School</th><th>Meal</th><th></th></tr></thead><tbody>`)
	for _, si := range sis {
		school := si.SchoolSlug
		if school == "" {
			school = "All schools"
		}
		meal := si.MealType
		if meal == "" {
			meal = "All meals"
		}
		sb.WriteString(fmt.Sprintf(
			`<tr><td><code>%s</code></td><td>%s</td><td>%s</td>`+
				`<td><button class="del-btn" data-id="%d" onclick="delSecInc(this)">Delete</button></td></tr>`,
			html.EscapeString(si.SectionName),
			html.EscapeString(school),
			html.EscapeString(meal),
			si.ID,
		))
	}
	sb.WriteString(`</tbody></table>`)
	return sb.String()
}

func buildSchoolOptions() string {
	var sb strings.Builder
	sb.WriteString(`<option value="">All schools</option>`)
	for _, s := range nutrislice.DefaultSchools {
		sb.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`,
			html.EscapeString(s.Slug), html.EscapeString(s.Name)))
	}
	return sb.String()
}

func truncURL(u string) string {
	if len(u) > 60 {
		return u[:58] + "…"
	}
	return u
}

const settingsPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Menu Settings</title>
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#F1F5F9;color:#1E293B;min-height:100vh}
    header{background:#0F172A;color:#fff;padding:1rem 1.5rem;display:flex;align-items:center;justify-content:space-between;gap:1rem}
    header h1{font-size:1.25rem;font-weight:700}
    .back-btn{color:#94A3B8;text-decoration:none;font-size:.85rem;border:1px solid #334155;padding:.35rem .8rem;border-radius:6px;transition:background .15s}
    .back-btn:hover{background:#1E293B;color:#fff}
    .page{max-width:900px;margin:1.5rem auto;padding:0 1rem;display:flex;flex-direction:column;gap:1.5rem}
    .card{background:#fff;border:1px solid #E2E8F0;border-radius:12px;overflow:hidden}
    .card-hdr{padding:.9rem 1.25rem;background:#F8FAFC;border-bottom:1px solid #E2E8F0}
    .card-hdr h2{font-size:1rem;font-weight:700;color:#0F172A}
    .card-hdr p{font-size:.78rem;color:#64748B;margin-top:.2rem}
    .card-body{padding:1.1rem 1.25rem;display:flex;flex-direction:column;gap:1rem}
    .data-table{width:100%;border-collapse:collapse;font-size:.82rem}
    .data-table th{text-align:left;font-size:.7rem;font-weight:700;text-transform:uppercase;letter-spacing:.06em;color:#64748B;padding:.4rem .6rem;border-bottom:2px solid #E2E8F0}
    .data-table td{padding:.5rem .6rem;border-bottom:1px solid #F1F5F9;vertical-align:middle}
    .data-table tr:last-child td{border-bottom:none}
    .prev-img{width:42px;height:42px;object-fit:cover;border-radius:6px}
    .url-cell{font-size:.75rem;color:#64748B;max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
    .url-cell a{color:#3B82F6;text-decoration:none}
    .url-cell a:hover{text-decoration:underline}
    .del-btn{background:#FEF2F2;border:1px solid #FECACA;color:#B91C1C;padding:.25rem .65rem;border-radius:6px;font-size:.75rem;font-weight:600;cursor:pointer;transition:background .15s}
    .del-btn:hover{background:#FEE2E2}
    .add-form{display:flex;gap:.5rem;flex-wrap:wrap;align-items:flex-end;padding-top:.5rem;border-top:1px solid #E2E8F0}
    .field{display:flex;flex-direction:column;gap:.25rem;flex:1;min-width:150px}
    .field label{font-size:.72rem;font-weight:600;color:#475569}
    .field input,.field select{padding:.4rem .65rem;border:1px solid #CBD5E1;border-radius:6px;font-size:.82rem;outline:none;transition:border .15s}
    .field input:focus,.field select:focus{border-color:#3B82F6}
    .add-btn{background:#3B82F6;border:none;color:#fff;padding:.4rem 1rem;border-radius:6px;font-size:.82rem;font-weight:600;cursor:pointer;white-space:nowrap;transition:background .15s}
    .add-btn:hover{background:#2563EB}
    .empty{color:#94A3B8;font-size:.82rem;font-style:italic}
    .warn{color:#92400E;background:#FFFBEB;border:1px solid #FDE68A;padding:.7rem 1rem;border-radius:8px;font-size:.85rem}
    code{background:#F1F5F9;padding:.1rem .3rem;border-radius:4px;font-size:.78rem}
    .si-sub-lbl{font-size:.72rem;font-weight:700;text-transform:uppercase;letter-spacing:.05em;color:#475569}
    .si-avail-btn{font-size:.78rem;padding:.25rem .6rem;border-radius:20px;border:1px solid #CBD5E1;background:#fff;cursor:pointer;transition:all .15s;color:#475569}
    .si-avail-btn:hover{border-color:#3B82F6;color:#3B82F6}
    .si-avail-btn.active{background:#3B82F6;border-color:#3B82F6;color:#fff}
    .si-chip{display:flex;align-items:center;gap:.35rem;background:#1E293B;border:1px solid #334155;color:#E2E8F0;padding:.3rem .65rem .3rem .45rem;border-radius:20px;font-size:.78rem;cursor:grab;user-select:none;transition:opacity .15s;position:relative}
    .si-chip:active{cursor:grabbing;opacity:.7}
    .si-chip.drag-over{border-color:#3B82F6;background:#1E3A5F}
    .si-chip-handle{color:#64748B;font-size:.9rem;margin-right:.1rem}
    .si-chip-del{background:none;border:none;color:#64748B;font-size:.8rem;cursor:pointer;padding:0 .1rem;line-height:1}
    .si-chip-del:hover{color:#EF4444}
    .si-pop{position:fixed;z-index:9999;background:#1E293B;border:1px solid #334155;border-radius:10px;padding:.7rem .9rem;max-width:240px;font-size:.75rem;color:#E2E8F0;box-shadow:0 8px 24px rgba(0,0,0,.3);pointer-events:none}
    .si-grid{display:grid;grid-template-columns:3.5rem auto auto;gap:.35rem .5rem;align-items:center}
    .si-row-lbl{font-size:.75rem;font-weight:700;color:#64748B;text-align:right;padding-right:.35rem;white-space:nowrap}
    .si-tab{font-size:.78rem;padding:.28rem .85rem;border-radius:20px;border:1px solid #CBD5E1;background:#fff;cursor:pointer;transition:all .15s;color:#475569;white-space:nowrap;font-family:inherit}
    .si-tab:hover{border-color:#3B82F6;color:#3B82F6}
    .si-tab.active{background:#3B82F6;border-color:#3B82F6;color:#fff}
    @media(max-width:640px){
      header{padding:.75rem 1rem}
      header h1{font-size:1.05rem}
      .page{padding:0 .5rem;margin:.85rem auto;gap:1rem}
      .card-hdr{padding:.75rem 1rem}
      .card-body{padding:.85rem 1rem}
      .add-form{flex-direction:column;gap:.65rem;align-items:stretch}
      .field{min-width:0;width:100%}
      .add-btn{width:100%;padding:.5rem 1rem}
      .data-table{font-size:.75rem}
      .url-cell{max-width:130px}
      .prev-img{width:32px;height:32px}
    }
  </style>
</head>
<body>
<header>
  <h1>⚙ Settings</h1>
  <a class="back-btn" href="/api">API</a>
  <a class="back-btn" href="/">← Calendar</a>
</header>

<div class="page">

  <!-- Custom Food Images -->
  <div class="card">
    <div class="card-hdr">
      <h2>Custom Food Images</h2>
      <p>Override or supply image URLs for foods not covered by the Nutrislice API.</p>
    </div>
    <div class="card-body">
      [[IMAGES_TABLE]]
      <form class="add-form" id="img-form">
        <div class="field">
          <label>Food Name</label>
          <input name="food_name" placeholder="e.g. Cheese Pizza" required>
        </div>
        <div class="field" style="flex:2">
          <label>Image URL</label>
          <input name="image_url" type="url" placeholder="https://..." required>
        </div>
        <button class="add-btn" type="submit">Add Image</button>
      </form>
    </div>
  </div>

  <!-- Favorites -->
  <div class="card">
    <div class="card-hdr">
      <h2>Favorites</h2>
      <p>Starred food items. Used to highlight meals in future UI features.</p>
    </div>
    <div class="card-body">
      [[FAVORITES_TABLE]]
      <form class="add-form" id="fav-form">
        <div class="field">
          <label>Food Name</label>
          <input name="food_name" placeholder="e.g. Pepperoni Pizza" required>
        </div>
        <div class="field">
          <label>School (optional)</label>
          <select name="school_slug">[[SCHOOL_OPTIONS]]</select>
        </div>
        <button class="add-btn" type="submit">Add Favorite</button>
      </form>
    </div>
  </div>

  <!-- Summary Exclusions -->
  <div class="card">
    <div class="card-hdr">
      <h2>Summary Exclusions</h2>
      <p>Patterns filtered from the <code>/api/v1/lunch/summary</code> endpoint. Case-insensitive substring match.</p>
    </div>
    <div class="card-body">
      [[EXCLUSIONS_TABLE]]
      <form class="add-form" id="ex-form">
        <div class="field">
          <label>Pattern</label>
          <input name="pattern" placeholder="e.g. sun butter" required>
        </div>
        <div class="field">
          <label>School (optional)</label>
          <select name="school_slug">[[SCHOOL_OPTIONS]]</select>
        </div>
        <button class="add-btn" type="submit">Add Exclusion</button>
      </form>
    </div>
  </div>

  <!-- Section Includes -->
  <div class="card">
    <div class="card-hdr">
      <h2>Section Include Rules</h2>
      <p>Control which sections appear and in what order. Pick a school and meal — select sections, then drag chips to reorder. Hover a chip to preview today's items.</p>
    </div>
    <div class="card-body">
      [[SECTION_INCLUDES_TABLE]]
      <div style="display:flex;flex-direction:column;gap:.8rem">
        <div class="si-grid">
          <div class="si-row-lbl">WRES</div>
          <button class="si-tab" data-school="woodmen-roberts-elementary-school" data-meal="breakfast" onclick="siPickTab(this)">Breakfast</button>
          <button class="si-tab" data-school="woodmen-roberts-elementary-school" data-meal="lunch" onclick="siPickTab(this)">Lunch</button>
          <div class="si-row-lbl">EMS</div>
          <button class="si-tab" data-school="eagleview-middle-school" data-meal="breakfast" onclick="siPickTab(this)">Breakfast</button>
          <button class="si-tab" data-school="eagleview-middle-school" data-meal="lunch" onclick="siPickTab(this)">Lunch</button>
        </div>
        <div id="si-panel" style="display:none;flex-direction:column;gap:.65rem">
          <div>
            <div class="si-sub-lbl">Available sections — click to toggle:</div>
            <div id="si-avail" style="display:flex;flex-wrap:wrap;gap:.35rem .5rem;margin-top:.35rem"></div>
          </div>
          <div>
            <div class="si-sub-lbl">Included order — drag to reorder:</div>
            <div id="si-chips" style="display:flex;flex-wrap:wrap;gap:.4rem;margin-top:.35rem;min-height:2.2rem;padding:.3rem;background:#F8FAFC;border:1px dashed #CBD5E1;border-radius:8px"></div>
          </div>
          <div style="display:flex;align-items:center;gap:.75rem">
            <button class="add-btn" onclick="siSave()">Save Order</button>
            <span id="si-status" style="font-size:.8rem;color:#64748B"></span>
          </div>
        </div>
        <div id="si-empty" style="font-size:.82rem;color:#94A3B8;font-style:italic;display:none">No cached data found. Visit the calendar for this school first to populate the cache.</div>
      </div>
      <div id="si-pop" class="si-pop" style="display:none"></div>
    </div>
  </div>

  <!-- Missing Images -->
  <div class="card">
    <div class="card-hdr">
      <h2>Missing Images</h2>
      <p>Scan cached menu data to find food items that have no image. Click an item to prefill the Add Image form above.</p>
    </div>
    <div class="card-body">
      <div style="display:flex;gap:.75rem;align-items:center;flex-wrap:wrap">
        <button class="add-btn" id="scan-btn" onclick="scanMissing()">Scan Cache</button>
        <span id="scan-status" style="font-size:.82rem;color:#64748B"></span>
      </div>
      <div id="missing-list"></div>
    </div>
  </div>

</div>

<script>
function delImage(btn) {
  fetch('/api/v1/food-images?food_name=' + encodeURIComponent(btn.dataset.name), {method:'DELETE'})
    .then(function(r){ if(r.ok||r.status===204) location.reload(); else alert('Error: '+r.status); });
}
function delFav(btn) {
  fetch('/api/v1/favorites?id=' + btn.dataset.id, {method:'DELETE'})
    .then(function(r){ if(r.ok||r.status===204) location.reload(); else alert('Error: '+r.status); });
}
function delExclusion(btn) {
  fetch('/api/v1/exclusions?id=' + btn.dataset.id, {method:'DELETE'})
    .then(function(r){ if(r.ok||r.status===204) location.reload(); else alert('Error: '+r.status); });
}
function delSecInc(btn) {
  fetch('/api/v1/section-includes?id=' + btn.dataset.id, {method:'DELETE'})
    .then(function(r){ if(r.ok||r.status===204) location.reload(); else alert('Error: '+r.status); });
}

document.getElementById('img-form').addEventListener('submit', function(e) {
  e.preventDefault();
  var f = e.target;
  fetch('/api/v1/food-images', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({food_name: f.food_name.value, image_url: f.image_url.value})
  }).then(function(r){ if(r.ok||r.status===204) location.reload(); else r.text().then(function(t){alert('Error: '+t);}); });
});

document.getElementById('fav-form').addEventListener('submit', function(e) {
  e.preventDefault();
  var f = e.target;
  fetch('/api/v1/favorites', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({food_name: f.food_name.value, school_slug: f.school_slug.value})
  }).then(function(r){ if(r.ok||r.status===204) location.reload(); else r.text().then(function(t){alert('Error: '+t);}); });
});

document.getElementById('ex-form').addEventListener('submit', function(e) {
  e.preventDefault();
  var f = e.target;
  fetch('/api/v1/exclusions', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({pattern: f.pattern.value, school_slug: f.school_slug.value})
  }).then(function(r){ if(r.ok||r.status===204) location.reload(); else r.text().then(function(t){alert('Error: '+t);}); });
});

// Prefill food_name from ?add_image= query param (linked from modal placeholders)
(function() {
  var search = window.location.search.slice(1);
  var addImg = '';
  var pairs = search.split('&');
  for (var i = 0; i < pairs.length; i++) {
    var kv = pairs[i].split('=');
    if (decodeURIComponent(kv[0]) === 'add_image') {
      addImg = decodeURIComponent((kv[1] || '').replace(/\+/g,' '));
      break;
    }
  }
  if (addImg) {
    var f = document.querySelector('#img-form [name="food_name"]');
    if (f) {
      f.value = addImg;
      setTimeout(function() {
        document.getElementById('img-form').scrollIntoView({behavior:'smooth', block:'center'});
        setTimeout(function() { f.focus(); }, 250);
      }, 150);
    }
  }
})();

// ── Section Includes: drag-chip UI ──────────────────────────────────────────
var siTodayCache = {};
var siDragSrc = null;
var siCurSchool = '';
var siCurMeal = '';

function siPickTab(btn) {
  document.querySelectorAll('.si-tab').forEach(function(t){ t.classList.remove('active'); });
  btn.classList.add('active');
  siCurSchool = btn.dataset.school;
  siCurMeal = btn.dataset.meal;
  siLoad();
}

function siLoad() {
  var school = siCurSchool;
  var meal = siCurMeal;
  var panel = document.getElementById('si-panel');
  var empty = document.getElementById('si-empty');
  panel.style.display = 'none'; empty.style.display = 'none';
  siTodayCache = {};
  if (!school) return;

  // Fetch both available sections and existing includes in parallel
  Promise.all([
    fetch('/api/v1/sections?school=' + encodeURIComponent(school) + '&meal=' + encodeURIComponent(meal)).then(function(r){ return r.json(); }),
    fetch('/api/v1/section-includes?school=' + encodeURIComponent(school) + '&meal=' + encodeURIComponent(meal)).then(function(r){ return r.json(); })
  ]).then(function(results) {
    var all = results[0] || [];
    var included = results[1] || [];
    if (!all.length) { empty.style.display = 'block'; return; }
    siRenderAvail(all, included);
    siRenderChips(included);
    panel.style.display = 'flex';
    document.getElementById('si-status').textContent = '';
  }).catch(function(){ empty.style.display = 'block'; });
}

function siRenderAvail(all, included) {
  var incSet = {};
  for (var i = 0; i < included.length; i++) incSet[included[i]] = true;
  var avail = document.getElementById('si-avail');
  avail.innerHTML = '';
  all.forEach(function(name) {
    var btn = document.createElement('button');
    btn.className = 'si-avail-btn' + (incSet[name] ? ' active' : '');
    btn.textContent = name;
    btn.onclick = function() { siToggle(name, btn); };
    avail.appendChild(btn);
  });
}

function siToggle(name, btn) {
  var chips = document.getElementById('si-chips');
  var existing = chips.querySelector('[data-sec="' + CSS.escape(name) + '"]');
  if (existing) {
    existing.remove();
    btn.classList.remove('active');
  } else {
    btn.classList.add('active');
    siAddChip(name);
  }
}

function siRenderChips(names) {
  var chips = document.getElementById('si-chips');
  chips.innerHTML = '';
  names.forEach(function(name) { siAddChip(name); });
}

function siAddChip(name) {
  var chips = document.getElementById('si-chips');
  var chip = document.createElement('div');
  chip.className = 'si-chip';
  chip.setAttribute('draggable', 'true');
  chip.setAttribute('data-sec', name);
  chip.innerHTML = '<span class="si-chip-handle">&#x2807;</span>' +
    '<span>' + escHtml(name) + '</span>' +
    '<button class="si-chip-del" title="Remove" onclick="siRemoveChip(this)">&times;</button>';

  chip.addEventListener('dragstart', function(e) {
    siDragSrc = chip;
    e.dataTransfer.effectAllowed = 'move';
  });
  chip.addEventListener('dragover', function(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    chip.classList.add('drag-over');
  });
  chip.addEventListener('dragleave', function() { chip.classList.remove('drag-over'); });
  chip.addEventListener('drop', function(e) {
    e.preventDefault();
    chip.classList.remove('drag-over');
    if (siDragSrc && siDragSrc !== chip) {
      chips.insertBefore(siDragSrc, chip);
    }
  });
  chip.addEventListener('dragend', function() { chip.classList.remove('drag-over'); });

  // Hover preview
  chip.addEventListener('mouseenter', function(e) { siShowPop(name, e); });
  chip.addEventListener('mousemove', function(e) { siMovePop(e); });
  chip.addEventListener('mouseleave', function() { siHidePop(); });

  chips.appendChild(chip);
}

function siRemoveChip(btn) {
  var chip = btn.parentElement;
  var name = chip.getAttribute('data-sec');
  chip.remove();
  var ab = document.querySelector('#si-avail .si-avail-btn.active');
  var avail = document.getElementById('si-avail');
  var btns = avail.querySelectorAll('.si-avail-btn');
  for (var i = 0; i < btns.length; i++) {
    if (btns[i].textContent === name) { btns[i].classList.remove('active'); break; }
  }
}

function siSave() {
  var school = siCurSchool;
  var meal = siCurMeal;
  var chips = document.getElementById('si-chips').querySelectorAll('.si-chip');
  var status = document.getElementById('si-status');
  if (!chips.length) { status.textContent = 'No sections selected.'; return; }
  var names = [];
  for (var i = 0; i < chips.length; i++) names.push(chips[i].getAttribute('data-sec'));
  status.textContent = 'Saving…';
  // POST any newly-added sections (existing ones ignored via INSERT OR IGNORE),
  // then reorder all of them by position in one PUT call.
  var seq = Promise.resolve();
  names.forEach(function(name) {
    seq = seq.then(function() {
      return fetch('/api/v1/section-includes', {
        method:'POST', headers:{'Content-Type':'application/json'},
        body: JSON.stringify({section_name: name, school_slug: school, meal_type: meal})
      });
    });
  });
  seq.then(function() {
    return fetch('/api/v1/section-includes/order', {
      method:'PUT', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({school_slug: school, meal_type: meal, sections: names})
    });
  }).then(function() { location.reload(); })
    .catch(function(){ status.textContent = 'Error saving.'; });
}

// Hover popover
function siShowPop(name, e) {
  var school = siCurSchool;
  var meal = siCurMeal;
  var pop = document.getElementById('si-pop');
  pop.style.display = 'block';
  pop.innerHTML = '<em style="color:#64748B">Loading today\'s ' + escHtml(name) + '…</em>';
  siMovePop(e);
  var key = school + ':' + meal;
  if (siTodayCache[key]) {
    siPopFill(pop, name, siTodayCache[key]);
    return;
  }
  fetch('/api/v1/lunch?school=' + encodeURIComponent(school) + '&date=today&meal=' + encodeURIComponent(meal))
    .then(function(r){ return r.json(); })
    .then(function(data) {
      siTodayCache[key] = data;
      siPopFill(pop, name, data);
    }).catch(function(){ pop.innerHTML = '<em style="color:#94A3B8">No data</em>'; });
}

function siPopFill(pop, name, data) {
  var secs = data && data.sections ? data.sections : [];
  var sec = null;
  for (var i = 0; i < secs.length; i++) {
    if (secs[i].Name && secs[i].Name.toLowerCase() === name.toLowerCase()) { sec = secs[i]; break; }
  }
  if (!sec || !sec.Foods || !sec.Foods.length) {
    pop.innerHTML = '<strong style="color:#93C5FD">' + escHtml(name) + '</strong><br><em style="color:#64748B">Nothing today</em>';
    return;
  }
  var h = '<strong style="color:#93C5FD">' + escHtml(name) + ' today:</strong><ul style="margin:.3rem 0 0 .9rem;line-height:1.6">';
  for (var i = 0; i < sec.Foods.length; i++) h += '<li>' + escHtml(sec.Foods[i].Name) + '</li>';
  pop.innerHTML = h + '</ul>';
}

function siMovePop(e) {
  var pop = document.getElementById('si-pop');
  var x = e.clientX + 14, y = e.clientY - 10;
  if (x + 250 > window.innerWidth) x = e.clientX - 260;
  pop.style.left = x + 'px';
  pop.style.top = y + 'px';
}

function siHidePop() {
  document.getElementById('si-pop').style.display = 'none';
}

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function scanMissing() {
  var btn = document.getElementById('scan-btn');
  var status = document.getElementById('scan-status');
  var list = document.getElementById('missing-list');
  btn.disabled = true;
  status.textContent = 'Scanning…';
  list.innerHTML = '';
  fetch('/api/v1/missing-images').then(function(r){ return r.json(); }).then(function(names) {
    btn.disabled = false;
    if (!names || names.length === 0) {
      status.textContent = 'No missing images found.';
      return;
    }
    status.textContent = names.length + ' food' + (names.length === 1 ? '' : 's') + ' without images:';
    var ul = document.createElement('ul');
    ul.style.cssText = 'list-style:none;display:flex;flex-direction:column;gap:.35rem;margin-top:.5rem';
    names.forEach(function(name) {
      var li = document.createElement('li');
      li.style.cssText = 'display:flex;align-items:center;gap:.5rem';
      var lbl = document.createElement('span');
      lbl.style.cssText = 'font-size:.82rem;color:#334155;flex:1';
      lbl.textContent = name;
      var abtn = document.createElement('button');
      abtn.className = 'add-btn';
      abtn.style.cssText = 'padding:.2rem .65rem;font-size:.75rem';
      abtn.textContent = '+ Add Image';
      abtn.onclick = function() {
        var fi = document.querySelector('#img-form [name="food_name"]');
        if (fi) {
          fi.value = name;
          document.getElementById('img-form').scrollIntoView({behavior:'smooth', block:'center'});
          setTimeout(function(){ fi.focus(); }, 250);
        }
      };
      li.appendChild(lbl);
      li.appendChild(abtn);
      ul.appendChild(li);
    });
    list.appendChild(ul);
  }).catch(function(err) {
    btn.disabled = false;
    status.textContent = 'Error: ' + err.message;
  });
}
</script>
</body>
</html>`
