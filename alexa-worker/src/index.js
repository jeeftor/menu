/**
 * Cloudflare Worker for the "Woodman Roberts School Lunch" Alexa skill.
 *
 * This worker does not call the homelab menu app. It fetches Nutrislice
 * directly, parses the weekly menu, and returns an Alexa response. No inbound
 * traffic to the LAN is required for the skill.
 *
 * Supported requests:
 *   - "open woodman roberts lunch"                    → welcome prompt
 *   - "what's for lunch today"                        → today's menu or "nothing today"
 *   - "what's for lunch tomorrow"                     → next school day's menu (skips weekends)
 *   - "help" / "stop" / "cancel"
 */

const TIME_ZONE = 'America/Denver';
const MAX_LOOKAHEAD = 14;
const CACHE_TTL_SECONDS = 3600;

export default {
  async fetch(request, env, ctx) {
    if (request.method !== 'POST') {
      return new Response('method not allowed', { status: 405 });
    }

    let body;
    try {
      body = await request.json();
    } catch {
      return new Response('bad request', { status: 400 });
    }

    // Basic sanity checks; full cert verification can be added later.
    const appId = body?.session?.application?.applicationId;
    if (appId !== env.SKILL_ID) {
      return new Response('unauthorized', { status: 401 });
    }
    const reqTs = body?.request?.timestamp;
    if (!reqTs || Math.abs(Date.now() - Date.parse(reqTs)) > 150_000) {
      return new Response('unauthorized', { status: 401 });
    }

    const response = await handleRequest(body, env, ctx);
    return new Response(JSON.stringify(response), {
      headers: { 'Content-Type': 'application/json' },
    });
  },
};

async function handleRequest(envEnvelope, env, ctx) {
  const request = envEnvelope.request;

  if (request.type === 'LaunchRequest') {
    return alexaResponse(
      'Ask me what\'s for lunch today or tomorrow.',
      false
    );
  }

  if (request.type === 'SessionEndedRequest') {
    return alexaResponse('', true);
  }

  if (request.type === 'IntentRequest') {
    const name = request.intent.name;

    if (name === 'AMAZON.HelpIntent') {
      return alexaResponse(
        'You can say what\'s for lunch today, or what\'s for lunch tomorrow.',
        false
      );
    }

    if (name === 'AMAZON.StopIntent' || name === 'AMAZON.CancelIntent') {
      return alexaResponse('Goodbye.', true);
    }

    if (name === 'MenuQueryIntent') {
      return await handleMenuQuery(request, env, ctx);
    }

    return alexaResponse(
      'Sorry, I can only tell you about lunch today or tomorrow.',
      true
    );
  }

  return alexaResponse('Sorry, I didn\'t understand.', true);
}

async function handleMenuQuery(request, env, ctx) {
  const slot = request.intent?.slots?.date;
  if (!slot || !slot.value) {
    return alexaResponse(
      'Please ask what\'s for lunch today or tomorrow.',
      false
    );
  }

  const slotDate = slot.value;
  const today = todayInZone(TIME_ZONE);
  const tomorrow = addDays(today, 1);

  if (slotDate === today) {
    const summary = await fetchDaySummary(today, env, ctx);
    if (!summary || !summary.text) {
      return alexaResponse('There is nothing on the menu today.', true);
    }
    return alexaResponse(`For lunch today, ${summary.text}.`, true);
  }

  if (slotDate === tomorrow) {
    const target = await nextSchoolDay(tomorrow, env, ctx);
    if (!target) {
      return alexaResponse(
        'There is nothing on the menu for the next few days.',
        true
      );
    }
    const weekday = formatWeekday(target.date);
    return alexaResponse(
      `Tomorrow is ${weekday}. The food will be ${target.text}.`,
      true
    );
  }

  return alexaResponse(
    'Right now I can only tell you about today or tomorrow.',
    false
  );
}

async function nextSchoolDay(startDate, env, ctx) {
  for (let i = 0; i < MAX_LOOKAHEAD; i++) {
    const candidate = addDays(startDate, i);
    const dt = new Date(candidate + 'T00:00:00');
    const dow = dt.getUTCDay();
    if (dow === 0 || dow === 6) {
      continue; // skip Saturday/Sunday
    }
    const summary = await fetchDaySummary(candidate, env, ctx);
    if (summary && summary.text) {
      return { date: candidate, text: summary.text };
    }
  }
  return null;
}

async function fetchDaySummary(date, env, ctx) {
  const parts = date.split('-').map((n) => parseInt(n, 10));
  if (parts.length !== 3 || parts.some(isNaN)) {
    return null;
  }
  const [year, month, day] = parts;

  const url = `https://${env.DISTRICT}.api.nutrislice.com/menu/api/weeks/school/${env.DEFAULT_SCHOOL}/menu-type/${env.DEFAULT_MEAL}/${year}/${month}/${day}/`;

  try {
    const data = await fetchWithCache(url, ctx);
    if (!data || !data.days) {
      return null;
    }

    const rawDay = data.days.find((d) => d.date === date);
    if (!rawDay || !rawDay.menu_items || rawDay.menu_items.length === 0) {
      return null;
    }

    return buildSummary(rawDay);
  } catch (err) {
    return null;
  }
}

async function fetchWithCache(url, ctx) {
  const cache = caches.default;
  const cacheReq = new Request(url, { headers: { Accept: 'application/json' } });

  let cached = await cache.match(cacheReq);
  if (cached) {
    try {
      return await cached.json();
    } catch {
      // stale/bad cache; fall through
    }
  }

  const resp = await fetch(url, { headers: { Accept: 'application/json' } });
  if (!resp.ok) {
    return null;
  }
  const text = await resp.text();

  const toCache = new Response(text, {
    status: 200,
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': `public, max-age=${CACHE_TTL_SECONDS}`,
    },
  });
  ctx.waitUntil(cache.put(cacheReq, toCache));

  return JSON.parse(text);
}

function buildSummary(rawDay) {
  const items = [...rawDay.menu_items].sort((a, b) => a.position - b.position);
  const sections = [];
  let current = null;

  for (const mi of items) {
    if (mi.is_section_title || mi.is_station_header) {
      sections.push({ name: mi.text, foods: [] });
      current = sections[sections.length - 1];
      continue;
    }
    if (!mi.food || !current) {
      continue;
    }
    current.foods.push(mi.food.name);
  }

  const optionSections = sections.filter(
    (s) => s.name && s.name.startsWith('Option')
  );

  const names = optionSections
    .map((s) => s.foods[0])
    .filter((n) => !!n);

  const text = joinNames(names);
  return { options: names, text };
}

function joinNames(names) {
  if (names.length === 0) return '';
  if (names.length === 1) return names[0];
  if (names.length === 2) return `${names[0]} or ${names[1]}`;
  return `${names.slice(0, -1).join(', ')}, or ${names[names.length - 1]}`;
}

function alexaResponse(text, shouldEndSession) {
  const response = {
    version: '1.0',
    response: {
      outputSpeech: { type: 'PlainText', text },
      shouldEndSession,
    },
  };
  if (!shouldEndSession) {
    response.response.reprompt = {
      type: 'PlainText',
      text: 'You can ask what\'s for lunch today or tomorrow.',
    };
  }
  return response;
}

function todayInZone(zone) {
  return new Date().toLocaleDateString('en-CA', { timeZone: zone });
}

function addDays(dateStr, n) {
  const dt = new Date(dateStr + 'T00:00:00');
  dt.setUTCDate(dt.getUTCDate() + n);
  return dt.toLocaleDateString('en-CA', { timeZone: 'UTC' });
}

function formatWeekday(dateStr) {
  const dt = new Date(dateStr + 'T00:00:00');
  return dt.toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    timeZone: 'UTC',
  });
}
