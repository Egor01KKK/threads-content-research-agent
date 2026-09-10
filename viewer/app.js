const app = document.querySelector("#app");

const DATA_URL = "/research/data/complete-readable.json";
const PREP_URL = "/research/data/video-prep.json";
const QA_USERNAME = "veuveh";
const DISPLAY_LIMIT = 72;

const state = {
  corpus: [],
  prep: null,
  proof: [],
  proofByURL: new Map(),
  proofByID: new Map(),
  recordByURL: new Map(),
  seedByUsername: new Map(),
  viewMode: "all",
  classFilter: "All",
  query: "",
  selectedURL: null,
  compareIDs: [],
  showDemo: false,
};

const numberFormatter = new Intl.NumberFormat("en-US");
const dateFormatter = new Intl.DateTimeFormat("ru-RU", {
  day: "2-digit",
  month: "short",
  year: "numeric",
});

const icons = {
  search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="6.8"></circle><path d="m16.2 16.2 4.3 4.3"></path></svg>',
  heart: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20.5 8.8c0 5.2-8.5 10.1-8.5 10.1S3.5 14 3.5 8.8A4.3 4.3 0 0 1 12 6.6a4.3 4.3 0 0 1 8.5 2.2Z"></path></svg>',
  reply: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 11.5a7.6 7.6 0 0 1-8 7.5 8.5 8.5 0 0 1-4.4-1.2L4 19l1.2-3.1A7.2 7.2 0 0 1 4 11.5 7.6 7.6 0 0 1 12 4a7.6 7.6 0 0 1 8 7.5Z"></path></svg>',
  repost: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m17 3 3 3-3 3"></path><path d="M20 6H8a4 4 0 0 0-4 4v1"></path><path d="m7 21-3-3 3-3"></path><path d="M4 18h12a4 4 0 0 0 4-4v-1"></path></svg>',
  quote: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8.5 11.5H4.3A2.3 2.3 0 0 1 2 9.2V6.3A2.3 2.3 0 0 1 4.3 4h4.2a2.3 2.3 0 0 1 2.3 2.3v5.2c0 4.6-2.5 7-6.9 8.5"></path><path d="M19.5 11.5h-4.2A2.3 2.3 0 0 1 13 9.2V6.3A2.3 2.3 0 0 1 15.3 4h4.2a2.3 2.3 0 0 1 2.3 2.3v5.2c0 4.6-2.5 7-6.9 8.5"></path></svg>',
  external: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 4h6v6"></path><path d="m20 4-9 9"></path><path d="M19 13v5a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5"></path></svg>',
  close: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18"></path></svg>',
  copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="8" y="8" width="11" height="12" rx="2"></rect><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h2"></path></svg>',
  alert: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4 3.2 19.5h17.6L12 4Z"></path><path d="M12 9v4"></path><path d="M12 16.8h.01"></path></svg>',
  chevron: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"></path></svg>',
};

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function formatNumber(value) {
  if (value === null || value === undefined || value === "") return "—";
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numberFormatter.format(numeric) : escapeHTML(value);
}

function formatDecimal(value, digits = 2) {
  if (value === null || value === undefined || value === "") return "—";
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric.toFixed(digits) : escapeHTML(value);
}

function formatRelative(value) {
  if (value === null || value === undefined || value === "") return "Not available";
  const numeric = Number(value);
  return Number.isFinite(numeric) ? `${numeric.toFixed(2)}×` : "Not available";
}

function formatDate(value) {
  if (!value) return "Date unavailable";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : dateFormatter.format(date);
}

function normalize(value) {
  return String(value ?? "").toLocaleLowerCase("ru-RU").trim();
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function firstPresent(...values) {
  return values.find((value) => value !== null && value !== undefined && value !== "") ?? "";
}

function postOf(record) {
  if (record?.post && typeof record.post === "object") return record.post;
  return record && typeof record === "object" ? record : {};
}

function normalizeRecord(record, index) {
  const source = record && typeof record === "object" ? record : {};
  const sourcePost = postOf(source);
  const post = { ...sourcePost };
  post.url = firstPresent(post.url, post.permalink, post.source_url, source.url, source.permalink);
  post.permalink = firstPresent(post.permalink, post.url);
  post.text = firstPresent(post.text, source.text);
  post.author_username = firstPresent(
    post.author_username,
    post.username,
    source.author_username,
    source.username,
  );
  post.published_at = firstPresent(post.published_at, post.timestamp, source.published_at);

  for (const [friendly, raw] of [
    ["likes", "like_count"],
    ["replies", "reply_count"],
    ["reposts", "repost_count"],
    ["quotes", "quote_count"],
  ]) {
    post[friendly] = firstPresent(post[friendly], post[raw], source[friendly], source[raw]);
  }

  const viewerKey = firstPresent(post.url, post.id, `${post.author_username || "record"}-${index + 1}`);
  return { ...source, post, _viewer_key: viewerKey };
}

function extractCorpus(payload) {
  if (Array.isArray(payload)) return payload;
  if (Array.isArray(payload?.corpus)) return payload.corpus;
  if (Array.isArray(payload?.posts)) return payload.posts;
  return null;
}

function displayLabel(value) {
  return String(value ?? "")
    .replaceAll("_", " ")
    .replaceAll("-", " ")
    .replace(/\s+/g, " ")
    .trim();
}

function rawValue(value) {
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "boolean") return value ? "true" : "false";
  return String(value);
}

function safeThreadsURL(value) {
  try {
    const parsed = new URL(value);
    if (parsed.protocol !== "https:") return null;
    const hostname = parsed.hostname.toLocaleLowerCase();
    if (!["threads.com", "www.threads.com", "threads.net", "www.threads.net"].includes(hostname)) return null;
    return parsed.toString();
  } catch {
    return null;
  }
}

function externalLink(url, label, className = "") {
  const safeURL = safeThreadsURL(url);
  if (!safeURL) return `<span class="${className} detail-value">Source URL unavailable</span>`;
  return `<a class="${className}" href="${escapeHTML(safeURL)}" target="_blank" rel="noopener noreferrer">${label} ${icons.external}</a>`;
}

function authorInitial(record) {
  const username = postOf(record).author_username || "?";
  return escapeHTML(username.slice(0, 1).toUpperCase());
}

function recordKey(record) {
  return record?._viewer_key || postOf(record).url || postOf(record).id || "";
}

function getSelectedRecord() {
  return state.recordByURL.get(state.selectedURL) || null;
}

function getProofForRecord(record) {
  return state.proofByURL.get(postOf(record).url) || null;
}

function getProfileSeed(username) {
  return state.seedByUsername.get(username) || null;
}

function getProfileContext(record) {
  const seed = getProfileSeed(postOf(record).author_username);
  return seed?.profile || null;
}

function getSearchHaystack(record) {
  const post = postOf(record);
  const username = post.author_username || "";
  const seed = getProfileSeed(username);
  const profile = seed?.profile || {};
  const assessment = record?.assessment || {};
  return normalize([
    post.text,
    username,
    profile.name,
    profile.bio,
    seed?.business_categories,
    seed?.anchor_queries,
    assessment.business_type,
    assessment.business_types,
    assessment.business_process,
    assessment.business_processes,
    assessment.tools_mentioned,
    assessment.pain_types,
  ].flat().join(" "));
}

function matchesSearch(record) {
  const tokens = normalize(state.query).split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return true;
  const haystack = getSearchHaystack(record);
  return tokens.every((token) => haystack.includes(token));
}

function matchesClass(proof, filter) {
  if (!proof || filter === "All") return filter === "All";
  const label = normalize(proof.evidence_class);
  if (filter === "Question") return label.includes("question");
  if (filter === "Hiring") return label.includes("hiring");
  if (filter === "Provider") return label.includes("provider");
  if (filter === "Workflow") return label.includes("workflow") || label.includes("process");
  if (filter === "Service search") return label.includes("service search") || label.includes("consumer recommendation");
  if (filter === "Performance") return proof.relative_performance !== null && proof.relative_performance !== undefined;
  return false;
}

function currentEntries() {
  let entries;
  if (state.viewMode === "video") {
    entries = state.proof
      .map((proof) => ({
        record: state.recordByURL.get(proof.url),
        proof,
      }))
      .filter((entry) => entry.record);
  } else {
    entries = state.corpus.map((record) => ({ record, proof: getProofForRecord(record) }));
  }

  return entries.filter((entry) => {
    if (!matchesSearch(entry.record)) return false;
    if (state.viewMode === "video" && !matchesClass(entry.proof, state.classFilter)) return false;
    return true;
  });
}

function evidenceBadge(proof) {
  if (!proof?.evidence_class) return "";
  return `<span class="evidence-badge">${escapeHTML(proof.evidence_class)}</span>`;
}

function relativeChip(record) {
  const value = record?.performance?.relative_performance;
  if (value === null || value === undefined) return "";
  const low = Number(value) < 1;
  return `<span class="relative-chip${low ? " is-low" : ""}">${escapeHTML(formatRelative(value))} baseline</span>`;
}

function metric(iconName, value, label) {
  return `<span class="metric" title="${escapeHTML(label)}">${icons[iconName]}<span>${formatNumber(value)}</span></span>`;
}

function renderPostCard(entry) {
  const record = entry.record;
  const proof = entry.proof;
  const post = postOf(record);
  const url = recordKey(record);
  const selected = state.selectedURL === url;
  const isQA = post.author_username === QA_USERNAME;
  const proofId = proof?.id || "";
  const compareAdded = proofId && state.compareIDs.includes(proofId);
  const postName = post.author_username || "unknown author";
  const displayName = getProfileContext(record)?.name || postName;
  const postText = post.text ? post.text : "Original text unavailable";
  const proofIDMarkup = state.viewMode === "video" && proofId
    ? `<span class="post-date">${escapeHTML(proofId)}</span>`
    : `<span class="post-date">${escapeHTML(formatDate(post.published_at))}</span>`;

  return `<article class="post-card${selected ? " is-selected" : ""}${isQA ? " is-qa" : ""}" tabindex="0" role="button" aria-label="Open post by @${escapeHTML(postName)}" data-record-url="${escapeHTML(url)}">
    <div class="post-card-top">
      <div class="author-line">
        <div class="author-initial">${authorInitial(record)}</div>
        <div class="author-copy">
          <div class="author-name">${escapeHTML(displayName)}</div>
          <div class="author-handle">@${escapeHTML(postName)}</div>
        </div>
      </div>
      <div class="card-actions">
        ${isQA ? '<span class="qa-badge">QA example</span>' : ""}
        ${evidenceBadge(proof)}
        ${proofIDMarkup}
      </div>
    </div>
    <div class="post-text">${escapeHTML(postText)}</div>
    <div class="post-card-bottom">
      <div class="metrics">
        ${metric("heart", post.likes, "Likes")}
        ${metric("reply", post.replies, "Replies")}
        ${metric("repost", post.reposts, "Reposts")}
        ${metric("quote", post.quotes, "Quotes")}
      </div>
      <div class="card-actions">
        ${state.viewMode === "video" && proofId ? `<button class="compare-link${compareAdded ? " is-added" : ""}" type="button" data-compare-id="${escapeHTML(proofId)}">${compareAdded ? "Added" : "+ Compare"}</button>` : ""}
        ${relativeChip(record)}
        ${externalLink(post.url, "Open in Threads ↗", "open-link")}
      </div>
    </div>
  </article>`;
}

function renderFeed() {
  const list = document.querySelector("#feed-list");
  const count = document.querySelector("#result-count");
  const footer = document.querySelector("#feed-footer");
  if (!list || !count || !footer) return;

  const entries = currentEntries();
  const visible = entries.slice(0, DISPLAY_LIMIT);
  count.textContent = `${numberFormatter.format(entries.length)} ${entries.length === 1 ? "result" : "results"}`;

  if (visible.length === 0) {
    list.innerHTML = `<div class="empty-state"><strong>No matching posts</strong><span>Try a word from the original text, an author, or a profile context.</span></div>`;
    footer.textContent = "";
  } else {
    list.innerHTML = visible.map(renderPostCard).join("");
    footer.textContent = entries.length > visible.length
      ? `Showing ${numberFormatter.format(visible.length)} of ${numberFormatter.format(entries.length)} matching records`
      : "End of the current view";
  }

  if (!entries.some((entry) => recordKey(entry.record) === state.selectedURL)) {
    state.selectedURL = entries[0] ? recordKey(entries[0].record) : null;
  }

  renderInspector();
  renderCompareDock();
}

function detailItem(key, value, wide = false) {
  return `<div class="detail-item${wide ? " is-wide" : ""}"><div class="detail-key">${escapeHTML(key)}</div><div class="detail-value">${escapeHTML(rawValue(value))}</div></div>`;
}

function renderDiscovery(record) {
  const provenance = asArray(record.provenance);
  const stages = asArray(record.stages);
  const stageMarkup = stages.length
    ? `<div class="stage-list">${stages.map((stage) => `<span class="stage-badge">${escapeHTML(stage)}</span>`).join("")}</div>`
    : `<div class="detail-value">No source stage stored</div>`;

  const provenanceMarkup = provenance.length
    ? provenance.map((item, index) => `<div class="detail-grid" style="margin-top: ${index ? "13px" : "10px"}; padding-top: ${index ? "13px" : "0"}; border-top: ${index ? "1px solid #eef0f3" : "0"};">
        ${detailItem("source_stage", item.source_stage)}
        ${detailItem("anchor_query", item.anchor_query)}
        ${detailItem("profile_username", item.profile_username)}
        ${detailItem("seed_post_id", item.seed_post_id)}
        ${detailItem("secondary_anchor", item.secondary_anchor)}
        ${detailItem("extracted_phrase", item.extracted_phrase)}
        ${detailItem("depth", item.depth)}
      </div>`).join("")
    : `<div class="detail-value" style="margin-top: 10px;">No provenance rows stored for this record.</div>`;

  return `<section class="inspector-section">
    <h3 class="section-label">Discovery</h3>
    ${stageMarkup}
    ${provenanceMarkup}
  </section>`;
}

function contextLabel(record, proof, seed) {
  if (seed?.verification_status === "accepted") return "Accepted operator context";
  if (proof?.context && normalize(proof.context).includes("provider")) return "Provider context";
  return "Search-stage author";
}

function renderAuthorContext(record, proof) {
  const username = postOf(record).author_username || "unknown-author";
  const seed = getProfileSeed(username);
  const profile = seed?.profile;
  const context = contextLabel(record, proof, seed);
  const seedMeta = seed
    ? `<div class="detail-grid" style="margin-top: 12px;">
        ${detailItem("verification_status", seed.verification_status)}
        ${detailItem("verification_confidence", seed.verification_confidence)}
        ${detailItem("profile_fetched", seed.profile_fetched)}
        ${detailItem("profile_posts", seed.profile_posts)}
        ${detailItem("business_categories", seed.business_categories, true)}
        ${detailItem("anchor_queries", seed.anchor_queries, true)}
      </div>`
    : `<div class="context-meta">No accepted-profile record is attached to this post.</div>`;

  const profileMarkup = profile
    ? `<div class="profile-name">${escapeHTML(profile.name || `@${username}`)}</div>
       <div class="profile-bio">${escapeHTML(profile.bio || "Bio unavailable")}</div>
       <div class="profile-meta"><span>followers <strong>${formatNumber(profile.follower_count)}</strong></span><span>Threads verified <strong>${rawValue(profile.verified)}</strong></span></div>`
    : "";

  return `<section class="inspector-section">
    <h3 class="section-label">Author context</h3>
    <div class="context-card">
      <div class="context-status"><span class="context-status-dot"></span>${escapeHTML(context)}</div>
      <div class="context-meta">Stored package context for <span class="detail-value">@${escapeHTML(username)}</span></div>
      ${profileMarkup}
      ${seedMeta}
    </div>
  </section>`;
}

function renderPerformance(record) {
  const performance = record.performance || {};
  const relative = performance.relative_performance;
  const baseline = performance.baseline_engagement;
  const baselineAvailable = baseline !== null && baseline !== undefined;
  const relativeAvailable = relative !== null && relative !== undefined;
  const coverage = performance.metric_coverage || {};

  return `<section class="inspector-section">
    <h3 class="section-label">Performance</h3>
    <div class="performance-grid">
      <div class="perf-cell"><div class="perf-label">Weighted engagement</div><div class="perf-value">${formatNumber(performance.engagement)}</div></div>
      <div class="perf-cell"><div class="perf-label">Author baseline</div><div class="perf-value${baselineAvailable ? "" : " is-muted"}">${baselineAvailable ? escapeHTML(String(baseline)) : "Not available"}</div></div>
      <div class="perf-cell"><div class="perf-label">Relative performance</div><div class="perf-value${relativeAvailable && Number(relative) >= 1 ? " is-positive" : " is-muted"}">${escapeHTML(formatRelative(relative))}</div></div>
      <div class="perf-cell"><div class="perf-label">Baseline confidence</div><div class="perf-value${performance.baseline_confidence === "usable" ? " is-positive" : " is-muted"}">${escapeHTML(rawValue(performance.baseline_confidence))}</div></div>
    </div>
    <div class="metric-coverage"><span>Metric coverage</span><strong>${escapeHTML(rawValue(coverage.known))} / ${escapeHTML(rawValue(coverage.total))} fields</strong></div>
    <span class="causal-note">descriptive, not causal</span>
  </section>`;
}

function assessmentRow(label, value, className = "") {
  return `<div class="assessment-row"><span>${escapeHTML(label)}</span><span class="${className}">${escapeHTML(rawValue(value))}</span></div>`;
}

function renderAssessment(record) {
  const post = postOf(record);
  const assessment = record.assessment || {};
  const reasons = asArray(post.relevance_reasons);
  const painTypes = asArray(assessment.pain_types);
  const tools = asArray(assessment.tools_mentioned);

  return `<section class="inspector-section">
    <h3 class="section-label">Previous system assessment</h3>
    <div class="assessment-panel">
      <div class="assessment-warning">${icons.alert}<span>Derived label — not source evidence. These are the stored deterministic fields, shown here so you can challenge them against the original text.</span></div>
      <div class="assessment-list">
        ${assessmentRow("Stored label", post.relevance_label, "stored-label")}
        ${assessmentRow("Relevance score", post.relevance_score)}
        ${assessmentRow("Pain strength", assessment.pain_strength)}
        ${assessmentRow("Solution seeking", assessment.solution_seeking)}
        ${assessmentRow("Buyer intent", assessment.buyer_intent)}
        ${assessmentRow("Owner likelihood", assessment.owner_likelihood)}
        ${assessmentRow("IT actionability", assessment.it_actionability)}
        ${assessmentRow("Business process", assessment.business_process)}
      </div>
      ${painTypes.length ? `<div class="tag-list">${painTypes.map((item) => `<span class="data-tag">${escapeHTML(item)}</span>`).join("")}</div>` : ""}
      ${tools.length ? `<div class="tag-list">${tools.map((item) => `<span class="data-tag">${escapeHTML(item)}</span>`).join("")}</div>` : ""}
      ${reasons.length ? `<div class="detail-key" style="margin-top: 12px;">stored relevance reasons</div><div class="detail-value">${escapeHTML(reasons.join(", "))}</div>` : ""}
    </div>
  </section>`;
}

function renderInspector() {
  const target = document.querySelector("#inspector-content");
  if (!target) return;
  const record = getSelectedRecord();
  if (!record) {
    target.innerHTML = `<div class="inspector-empty">Select a post to inspect its source, provenance, and stored fields.</div>`;
    return;
  }

  const post = postOf(record);
  const proof = getProofForRecord(record);
  const isQA = post.author_username === QA_USERNAME && state.proof.length > 0;
  const displayName = getProfileContext(record)?.name || `@${post.author_username}`;
  const badge = isQA ? '<span class="qa-badge">QA example</span>' : evidenceBadge(proof);
  const sourceText = post.text ? post.text : "Original text unavailable";

  target.innerHTML = `<section class="inspector-section">
    <h3 class="section-label">Original source</h3>
    <div class="source-heading">
      <div><h2 class="source-author">@${escapeHTML(post.author_username)}</h2><div class="source-handle">${escapeHTML(displayName)}</div><div class="source-date">${escapeHTML(formatDate(post.published_at))}</div></div>
      ${badge}
    </div>
    <div class="source-text">${escapeHTML(sourceText)}</div>
    ${externalLink(post.url, "Open original ↗", "primary-button source-open")}
  </section>
  ${isQA ? `<section class="inspector-section"><div class="qa-reveal"><div><div class="qa-reveal-label">Original text</div><div class="qa-reveal-text">${escapeHTML(sourceText)}</div></div><div><div class="qa-reveal-label">Stored system label</div><div class="qa-reveal-value">${escapeHTML(rawValue(post.relevance_label))}</div></div></div></section>` : ""}
  ${renderDiscovery(record)}
  ${renderAuthorContext(record, proof)}
  ${renderPerformance(record)}
  ${renderAssessment(record)}`;
}

function contrastFor(proofA, proofB) {
  if (!proofA || !proofB) return null;
  return asArray(state.prep?.contrast_pairs).find((pair) => (
    (pair.post_a === proofA.id && pair.post_b === proofB.id) ||
    (pair.post_a === proofB.id && pair.post_b === proofA.id)
  )) || null;
}

function compareItem(proof) {
  const record = state.recordByURL.get(proof.url);
  const text = postOf(record).text || "";
  return `<div class="compare-item"><div class="compare-item-head"><div class="compare-item-author">@${escapeHTML(proof.author)}</div><div class="compare-item-id">${escapeHTML(proof.id)}</div></div><div class="compare-item-text">${escapeHTML(text)}</div><div class="tag-list"><span class="evidence-badge">${escapeHTML(proof.evidence_class)}</span></div></div>`;
}

function renderCompareDock() {
  const dock = document.querySelector("#compare-dock");
  if (!dock) return;
  if (state.compareIDs.length === 0) {
    dock.classList.add("is-hidden");
    dock.innerHTML = "";
    return;
  }

  const proofs = state.compareIDs.map((id) => state.proofByID.get(id)).filter(Boolean);
  const pair = proofs.length === 2 ? contrastFor(proofs[0], proofs[1]) : null;
  const slots = proofs.map(compareItem);
  if (proofs.length === 1) slots.push(`<div class="compare-item"><div class="compare-item-author" style="color: var(--muted);">Select one more proof post</div><div class="compare-item-text">Use + Compare on another card to open a package-defined contrast.</div></div>`);

  dock.classList.remove("is-hidden");
  dock.innerHTML = `<div class="compare-dock-header"><h3 class="compare-dock-title">Compare proof posts · ${proofs.length}/2</h3><div class="compare-dock-actions"><button class="quiet-button" type="button" data-action="clear-compare">Clear</button><button class="quiet-button" type="button" data-action="close-compare">${icons.close}</button></div></div><div class="compare-grid">${slots.join("")}</div>${proofs.length === 2 ? `<div class="compare-pair-note">${pair ? `<strong>${escapeHTML(pair.title)}</strong> — ${escapeHTML(pair.safe_point)}` : "This pair is not a pre-defined contrast in video-prep.json."}</div>` : ""}`;
}

function renderDemoPopover() {
  const popover = document.querySelector("#demo-popover");
  if (!popover) return;
  if (!state.showDemo) {
    popover.classList.add("is-hidden");
    return;
  }
  const command = state.prep?.demo?.primary_command || "Command unavailable";
  popover.classList.remove("is-hidden");
  popover.innerHTML = `<div class="demo-popover-header"><h3 class="demo-popover-title">Live CLI demo</h3><button class="quiet-button" type="button" data-action="close-demo">${icons.close}</button></div><p class="demo-popover-note">This only shows or copies the package command. The viewer does not run live collection.</p><code id="demo-command" class="command-block">${escapeHTML(command)}</code><div class="demo-actions"><button class="outline-button" type="button" data-action="copy-demo">${icons.copy} Copy command</button></div>`;
}

function setSelected(url) {
  state.selectedURL = url;
  renderFeed();
}

function setViewMode(mode) {
  state.viewMode = mode;
  state.classFilter = "All";
  const select = document.querySelector("#class-filter");
  if (select) {
    select.value = "All";
    select.disabled = mode !== "video";
  }
  const entries = currentEntries();
  state.selectedURL = entries[0] ? recordKey(entries[0].record) : null;
  document.querySelectorAll(".view-tab").forEach((button) => button.classList.toggle("is-active", button.dataset.view === mode));
  renderFeed();
}

function openQAExample() {
  state.viewMode = "all";
  state.classFilter = "All";
  state.query = QA_USERNAME;
  const search = document.querySelector("#search-input");
  if (search) search.value = QA_USERNAME;
  const select = document.querySelector("#class-filter");
  if (select) {
    select.value = "All";
    select.disabled = true;
  }
  const qaRecord = state.corpus.find((record) => postOf(record).author_username === QA_USERNAME);
  state.selectedURL = qaRecord ? recordKey(qaRecord) : null;
  document.querySelectorAll(".view-tab").forEach((button) => button.classList.toggle("is-active", button.dataset.view === "all"));
  renderFeed();
}

function toggleCompare(id) {
  if (!id) return;
  if (state.compareIDs.includes(id)) {
    state.compareIDs = state.compareIDs.filter((item) => item !== id);
  } else if (state.compareIDs.length < 2) {
    state.compareIDs = [...state.compareIDs, id];
  } else {
    state.compareIDs = [state.compareIDs[1], id];
  }
  renderFeed();
}

async function copyText(value) {
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    const area = document.createElement("textarea");
    area.value = value;
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    const copied = document.execCommand("copy");
    area.remove();
    return copied;
  }
}

function attachEvents() {
  const search = document.querySelector("#search-input");
  search.addEventListener("input", () => {
    state.query = search.value;
    renderFeed();
  });

  document.querySelectorAll(".view-tab").forEach((button) => {
    button.addEventListener("click", () => setViewMode(button.dataset.view));
  });

  document.querySelector("#class-filter").addEventListener("change", (event) => {
    state.classFilter = event.target.value;
    const entries = currentEntries();
    if (!entries.some((entry) => recordKey(entry.record) === state.selectedURL)) state.selectedURL = entries[0] ? recordKey(entries[0].record) : null;
    renderFeed();
  });

  const qaButton = document.querySelector("#qa-button");
  if (qaButton) qaButton.addEventListener("click", openQAExample);
  const demoButton = document.querySelector("#demo-button");
  if (demoButton) {
    demoButton.addEventListener("click", () => {
      state.showDemo = !state.showDemo;
      renderDemoPopover();
    });
  }

  document.addEventListener("click", async (event) => {
    const compareButton = event.target.closest("[data-compare-id]");
    if (compareButton) {
      event.preventDefault();
      event.stopPropagation();
      toggleCompare(compareButton.dataset.compareId);
      return;
    }

    const action = event.target.closest("[data-action]")?.dataset.action;
    if (action === "close-demo") {
      state.showDemo = false;
      renderDemoPopover();
      return;
    }
    if (action === "copy-demo") {
      const command = state.prep?.demo?.primary_command || "";
      const button = event.target.closest("[data-action]");
      const copied = await copyText(command);
      if (button) {
        button.innerHTML = copied ? "Copied" : "Press Cmd/Ctrl+C";
        window.setTimeout(() => { if (button.isConnected) button.innerHTML = `${icons.copy} Copy command`; }, 1500);
      }
      return;
    }
    if (action === "clear-compare" || action === "close-compare") {
      state.compareIDs = [];
      renderFeed();
      return;
    }

    const card = event.target.closest("[data-record-url]");
    if (card && !event.target.closest("a, button")) {
      setSelected(card.dataset.recordUrl);
    }
  });

  document.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === "k") {
      event.preventDefault();
      search.focus();
      search.select();
    }
    const card = event.target.closest?.("[data-record-url]");
    if (card && (event.key === "Enter" || event.key === " ")) {
      event.preventDefault();
      setSelected(card.dataset.recordUrl);
    }
  });
}

function renderShell() {
  const uniqueAuthors = new Set(state.corpus.map((record) => postOf(record).author_username).filter(Boolean)).size;
  const proofCount = state.proof.length;
  const hasQA = state.proof.length > 0 && state.corpus.some((record) => postOf(record).author_username === QA_USERNAME);
  const hasDemo = Boolean(state.prep?.demo?.primary_command);
  app.innerHTML = `<header class="topbar">
    <div class="brand"><div class="brand-mark">TR</div><div><div class="brand-name">Threads Research</div><div class="brand-context">Read-only dataset view</div></div></div>
    <label class="global-search" for="search-input">${icons.search}<input id="search-input" type="search" autocomplete="off" spellcheck="false" placeholder="Search original text, author, or profile context…" aria-label="Search original text, author, or profile context" /><span class="search-shortcut">⌘ K</span></label>
    <div class="dataset-indicator"><span class="dataset-dot"></span><span>${numberFormatter.format(state.corpus.length)} posts · ${numberFormatter.format(uniqueAuthors)} authors</span></div>
    <div class="topbar-actions">${hasDemo ? '<button id="demo-button" class="quiet-button" type="button">CLI demo</button>' : ""}</div>
  </header>
  <main class="main-layout">
    <section class="results-column" aria-label="Research results">
      <div class="results-header">
        <div class="results-heading-row"><div><p class="eyebrow">Public Threads dataset</p><h1 class="results-title">The original post is the evidence.</h1><p class="results-subtitle">Browse collected records, then inspect the source text, metrics, provenance, and any derived fields stored with the export.</p></div><div id="result-count" class="result-count"></div></div>
        <div class="view-toolbar"><div class="view-tabs" role="tablist" aria-label="Corpus view"><button class="view-tab is-active" type="button" role="tab" data-view="all">All posts <span class="tab-count">${numberFormatter.format(state.corpus.length)}</span></button>${proofCount ? `<button class="view-tab" type="button" role="tab" data-view="video">Video proof <span class="tab-count">${numberFormatter.format(proofCount)}</span></button>` : ""}</div><div class="toolbar-right"><select id="class-filter" class="filter-select" aria-label="Video proof class filter" disabled><option>All</option><option>Question</option><option>Hiring</option><option>Provider</option><option>Workflow</option><option>Service search</option><option>Performance</option></select>${hasQA ? '<button id="qa-button" class="outline-button" type="button">Open QA example</button>' : ""}</div></div>
      </div>
      <div id="feed-scroll" class="feed-scroll"><div id="feed-list" class="feed-list"></div><div id="feed-footer" class="feed-footer"></div></div>
    </section>
    <aside class="inspector-column" aria-label="Evidence inspector"><div class="inspector-header"><h2 class="inspector-title">Evidence inspector</h2><span class="inspector-kicker">source first</span></div><div id="inspector-content" class="inspector-scroll"></div></aside>
  </main>
  <div id="demo-popover" class="demo-popover is-hidden" aria-live="polite"></div>
  <div id="compare-dock" class="compare-dock is-hidden" aria-live="polite"></div>`;
}

function hydrate(payload, prep) {
  state.corpus = asArray(extractCorpus(payload)).map(normalizeRecord);
  state.prep = prep && typeof prep === "object" ? prep : {};
  state.seedByUsername.clear();
  state.proof = asArray(state.prep.proof_threads);
  state.proofByURL = new Map(state.proof.map((proof) => [proof.url, proof]));
  state.proofByID = new Map(state.proof.map((proof) => [proof.id, proof]));
  state.recordByURL = new Map(state.corpus.map((record) => [recordKey(record), record]));

  const seeds = asArray(payload.seed_profiles).filter((seed) => seed.username);
  for (const seed of seeds) {
    const existing = state.seedByUsername.get(seed.username);
    if (!existing || seed.verification_status === "accepted" || seed.selected) state.seedByUsername.set(seed.username, seed);
  }

  state.selectedURL = state.corpus[0] ? recordKey(state.corpus[0]) : null;
  renderShell();
  attachEvents();
  renderFeed();
  renderDemoPopover();
}

async function boot() {
  try {
    const corpusResponse = await fetch(DATA_URL);
    if (!corpusResponse.ok) throw new Error("The selected dataset could not be loaded.");
    const payload = await corpusResponse.json();
    if (!extractCorpus(payload)) throw new Error("The dataset must contain a corpus or posts array.");

    let prep = {};
    const prepResponse = await fetch(PREP_URL);
    if (prepResponse.ok) {
      const candidate = await prepResponse.json();
      if (candidate && typeof candidate === "object" && !Array.isArray(candidate)) prep = candidate;
    }
    hydrate(payload, prep);
  } catch (error) {
    app.innerHTML = `<div class="error-state"><div class="loading-mark">TR</div><strong>Could not load the selected dataset</strong><p>${escapeHTML(error.message)}</p><code class="command-block">python3 scripts/research-viewer.py --input examples/sample-export.json</code></div>`;
  }
}

boot();
