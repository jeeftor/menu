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
      <p>Choose which sections to show for a school+meal. Pick a school and meal below — sections found in the cache will appear as checkboxes.</p>
    </div>
    <div class="card-body">
      [[SECTION_INCLUDES_TABLE]]
      <div class="add-form" style="flex-direction:column;gap:.75rem">
        <div style="display:flex;gap:.5rem;flex-wrap:wrap;align-items:flex-end">
          <div class="field">
            <label>School</label>
            <select id="si-school" onchange="siLoadSections()">
              <option value="">Pick a school…</option>
              [[SCHOOL_OPTIONS]]
            </select>
          </div>
          <div class="field">
            <label>Meal Type</label>
            <select id="si-meal" onchange="siLoadSections()">
              <option value="lunch">Lunch</option>
              <option value="breakfast">Breakfast</option>
            </select>
          </div>
        </div>
        <div id="si-sections" style="display:none">
          <div style="font-size:.75rem;font-weight:600;color:#475569;margin-bottom:.4rem">Sections found in cache — check to include:</div>
          <div id="si-checklist" style="display:flex;flex-wrap:wrap;gap:.4rem .75rem"></div>
          <button class="add-btn" style="margin-top:.65rem" onclick="siSaveChecked()">Save Selected</button>
          <span id="si-status" style="font-size:.8rem;color:#64748B;margin-left:.5rem"></span>
        </div>
        <div id="si-empty" style="font-size:.82rem;color:#94A3B8;font-style:italic;display:none">No cached data found for this school+meal. Visit the calendar for that school first to populate the cache.</div>
      </div>
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

function siLoadSections() {
  var school = document.getElementById('si-school').value;
  var meal = document.getElementById('si-meal').value;
  var sectionsDiv = document.getElementById('si-sections');
  var emptyDiv = document.getElementById('si-empty');
  var checklist = document.getElementById('si-checklist');
  if (!school) { sectionsDiv.style.display='none'; emptyDiv.style.display='none'; return; }
  sectionsDiv.style.display='none'; emptyDiv.style.display='none';
  checklist.innerHTML = '<em style="color:#64748B;font-size:.78rem">Loading…</em>';
  fetch('/api/v1/sections?school=' + encodeURIComponent(school) + '&meal=' + encodeURIComponent(meal))
    .then(function(r){ return r.json(); })
    .then(function(names) {
      checklist.innerHTML = '';
      if (!names || names.length === 0) { emptyDiv.style.display='block'; return; }
      names.forEach(function(name) {
        var lbl = document.createElement('label');
        lbl.style.cssText = 'display:flex;align-items:center;gap:.3rem;font-size:.82rem;cursor:pointer;padding:.2rem .5rem;border:1px solid #E2E8F0;border-radius:6px;background:#F8FAFC';
        var cb = document.createElement('input');
        cb.type = 'checkbox'; cb.value = name;
        lbl.appendChild(cb);
        lbl.appendChild(document.createTextNode(name));
        checklist.appendChild(lbl);
      });
      sectionsDiv.style.display='block';
      document.getElementById('si-status').textContent = '';
    })
    .catch(function(){ emptyDiv.style.display='block'; });
}

function siSaveChecked() {
  var school = document.getElementById('si-school').value;
  var meal = document.getElementById('si-meal').value;
  var checks = document.querySelectorAll('#si-checklist input[type=checkbox]:checked');
  if (!checks.length) { document.getElementById('si-status').textContent = 'Nothing checked.'; return; }
  var status = document.getElementById('si-status');
  status.textContent = 'Saving…';
  var promises = [];
  for (var i = 0; i < checks.length; i++) {
    promises.push(fetch('/api/v1/section-includes', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({section_name: checks[i].value, school_slug: school, meal_type: meal})
    }));
  }
  Promise.all(promises).then(function(results) {
    var ok = results.every(function(r){ return r.ok || r.status === 204 || r.status === 409; });
    if (ok) { location.reload(); } else { status.textContent = 'Some saves failed.'; }
  }).catch(function(){ status.textContent = 'Error saving.'; });
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
