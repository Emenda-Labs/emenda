import { useState } from "react";

const C = {
  entry:    { bg: "#0a1628", border: "#3b82f6", accent: "#60a5fa", dim: "rgba(59,130,246,0.15)" },
  scan:     { bg: "#0a1f18", border: "#10b981", accent: "#34d399", dim: "rgba(16,185,129,0.12)" },
  artifact: { bg: "#1a180a", border: "#f59e0b", accent: "#fbbf24", dim: "rgba(245,158,11,0.15)" },
  decision: { bg: "#160a20", border: "#a855f7", accent: "#c084fc", dim: "rgba(168,85,247,0.15)" },
  rf:       { bg: "#0f1a0a", border: "#84cc16", accent: "#a3e635", dim: "rgba(132,204,22,0.15)" },
  ai:       { bg: "#200a0a", border: "#ef4444", accent: "#f87171", dim: "rgba(239,68,68,0.15)" },
  output:   { bg: "#0a1628", border: "#818cf8", accent: "#a5b4fc", dim: "rgba(129,140,248,0.15)" },
};

const nodes = {
  cli: {
    id: "cli", color: "entry", icon: "⌘", label: "CLI Input",
    sub: "--module  --to  --repo",
    desc: "Единственная точка входа. Три параметра: путь к репо, модуль, целевая версия.",
  },
  scan: {
    id: "scan", color: "scan", icon: "⌕", label: "Repo Scanner",
    sub: "Рекурсивный обход .go файлов → список импортирующих модуль",
    desc: "Обходит все .go файлы репо, ищет import пути совпадающие с целевым модулем. Выходной артефакт — список файлов и используемые символы.",
  },
  astdiff: {
    id: "astdiff", color: "scan", icon: "⊕", label: "AST Diff Engine",
    sub: "Клонирует dep · парсит оба тега · сравнивает экспорты",
    desc: "Клонирует репо зависимости, чекаутит from/to версии. Семантическое сравнение экспортированных деклараций через go/ast.",
  },
  changespec: {
    id: "changespec", color: "artifact", icon: "◈", label: "Change Spec",
    sub: "JSON: renames · removed · signature changes",
    desc: "Структурированный артефакт из AST diff. Типизированные изменения: rename_func, rename_field, rename_type, rename_method, remove_func.",
  },
  intersect: {
    id: "intersect", color: "decision", icon: "⋈", label: "Impact Intersection",
    sub: "Файлы × Change Spec → точная карта что сломано где",
    desc: "Пересекает список файлов из Scanner с Change Spec. Определяет какие символы в каждом файле затронуты. Файлы без пересечений выпадают.",
  },
  importresolver: {
    id: "importresolver", color: "scan", icon: "◎", label: "Import Resolver",
    sub: "Читает AST файлов → находит алиасы импортов",
    desc: "Определяет как именно импортирован модуль в каждом файле — с алиасом или без. Адаптирует имена под локальные алиасы чтобы rf скрипт был корректным.",
  },
  scriptgen: {
    id: "scriptgen", color: "rf", icon: "✎", label: "Script Generator",
    sub: "Change Spec → rf script",
    desc: "Маппинг known patterns на rf команды: rename_func → mv OldFunc NewFunc, remove_func → rm Func. Генерирует готовый rf скрипт.",
  },
  rf_known: {
    id: "rf_known", color: "rf", icon: "⚙", label: "rf — path 1",
    sub: "Known patterns · пишет файлы напрямую",
    desc: "Получает скрипт от Script Generator. Применяет механические трансформации и пишет изменения прямо на диск. Никакого промежуточного слоя.",
  },
  ai: {
    id: "ai", color: "ai", icon: "✦", label: "AI Agent",
    sub: "Unknown patterns · генерирует новый rf скрипт",
    desc: "Получает минимальный контекст: файл, сломанные символы, change spec. Не пишет код напрямую — генерирует новый rf скрипт для сложных трансформаций которые не покрываются известными паттернами.",
  },
  rf_ai: {
    id: "rf_ai", color: "rf", icon: "⚙", label: "rf — path 2",
    sub: "AI-generated script · пишет файлы напрямую",
    desc: "Получает скрипт сгенерированный AI агентом. Применяет трансформации и пишет изменения на диск. Тот же rf, тот же механизм записи — только источник скрипта другой.",
  },
  pr: {
    id: "pr", color: "output", icon: "↗", label: "Pull Request",
    sub: "Path 1 commits · Path 2 commits · TODO list",
    desc: "Создаёт ветку. Коммиты разделены по пути: механические (path 1) и AI-generated (path 2) отдельно. Описание PR: auto-fixed, AI-fixed, TODO для неприменённых изменений. CI решает.",
  },
};

function Node({ node, active, onClick }) {
  const col = C[node.color];
  const isActive = active === node.id;
  return (
    <div
      onClick={() => onClick(node.id)}
      style={{
        background: col.bg,
        border: `1px solid ${isActive ? col.accent : col.border}`,
        borderRadius: "8px",
        padding: "12px 16px",
        cursor: "pointer",
        transition: "all 0.18s ease",
        boxShadow: isActive ? `0 0 20px ${col.dim}, 0 0 0 1px ${col.accent}` : `0 2px 10px ${col.dim}`,
        width: "252px",
        userSelect: "none",
        flexShrink: 0,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: "8px", marginBottom: "3px" }}>
        <span style={{ color: col.accent, fontSize: "14px" }}>{node.icon}</span>
        <span style={{ color: col.accent, fontFamily: "'Space Mono', monospace", fontSize: "11.5px", fontWeight: 700, letterSpacing: "0.04em" }}>
          {node.label}
        </span>
      </div>
      <div style={{ color: "#6b7280", fontSize: "10px", fontFamily: "'Space Mono', monospace", lineHeight: 1.5, paddingLeft: "22px" }}>
        {node.sub}
      </div>
      {isActive && (
        <div style={{
          marginTop: "10px", paddingTop: "10px",
          borderTop: `1px solid ${col.border}`,
          color: "#d1d5db", fontSize: "10.5px",
          fontFamily: "Georgia, serif", lineHeight: 1.7,
          paddingLeft: "22px",
        }}>
          {node.desc}
        </div>
      )}
    </div>
  );
}

function Arrow({ color, label }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: "1px", padding: "1px 0" }}>
      {label && <span style={{ color: "#4b5563", fontSize: "9px", fontFamily: "'Space Mono', monospace" }}>{label}</span>}
      <svg width="14" height="20" viewBox="0 0 14 20">
        <line x1="7" y1="0" x2="7" y2="14" stroke={color} strokeWidth="1.5" />
        <polygon points="7,20 3,12 11,12" fill={color} />
      </svg>
    </div>
  );
}

// Splits from center to two columns
function SplitDown({ leftColor, rightColor, leftLabel, rightLabel, w = 580 }) {
  const cx = w / 2;
  const lx = cx - 145;
  const rx = cx + 145;
  return (
    <div style={{ position: "relative", width: w, height: 52 }}>
      <svg width={w} height={52} viewBox={`0 0 ${w} 52`} style={{ position: "absolute" }}>
        <path d={`M${cx},0 L${cx},18 L${lx},18 L${lx},52`} stroke={leftColor}  strokeWidth="1.5" fill="none" />
        <path d={`M${cx},0 L${cx},18 L${rx},18 L${rx},52`} stroke={rightColor} strokeWidth="1.5" fill="none" />
        <polygon points={`${lx},52 ${lx-4},44 ${lx+4},44`} fill={leftColor} />
        <polygon points={`${rx},52 ${rx-4},44 ${rx+4},44`} fill={rightColor} />
      </svg>
      {leftLabel  && <span style={{ position: "absolute", left:  "7%",  top: "21px", color: leftColor,  fontSize: "9px", fontFamily: "'Space Mono',monospace" }}>{leftLabel}</span>}
      {rightLabel && <span style={{ position: "absolute", right: "5%", top: "21px", color: rightColor, fontSize: "9px", fontFamily: "'Space Mono',monospace" }}>{rightLabel}</span>}
    </div>
  );
}

// Two columns converge to center
function MergeUp({ leftColor, rightColor, w = 580 }) {
  const cx = w / 2;
  const lx = cx - 145;
  const rx = cx + 145;
  return (
    <div style={{ position: "relative", width: w, height: 48 }}>
      <svg width={w} height={48} viewBox={`0 0 ${w} 48`} style={{ position: "absolute" }}>
        <path d={`M${lx},0 L${lx},24 L${cx},24 L${cx},48`} stroke={leftColor}  strokeWidth="1.5" fill="none" />
        <path d={`M${rx},0 L${rx},24 L${cx},24`}            stroke={rightColor} strokeWidth="1.5" fill="none" />
        <polygon points={`${cx},48 ${cx-4},40 ${cx+4},40`} fill={C.output.accent} />
      </svg>
    </div>
  );
}

// Straight parallel: two nodes in columns, each with arrow above
function ParallelArrows({ leftColor, rightColor }) {
  return (
    <div style={{ display: "flex", gap: "82px", justifyContent: "center" }}>
      <Arrow color={leftColor} />
      <Arrow color={rightColor} />
    </div>
  );
}

function SplitParallel({ color, w = 580 }) {
  const cx = w / 2;
  const lx = cx - 145;
  const rx = cx + 145;
  return (
    <div style={{ position: "relative", width: w, height: 44 }}>
      <svg width={w} height={44} viewBox={`0 0 ${w} 44`} style={{ position: "absolute" }}>
        <path d={`M${cx},0 L${cx},16 L${lx},16 L${lx},44`} stroke={color} strokeWidth="1.5" fill="none" />
        <path d={`M${cx},0 L${cx},16 L${rx},16 L${rx},44`} stroke={color} strokeWidth="1.5" fill="none" />
        <polygon points={`${lx},44 ${lx-4},36 ${lx+4},36`} fill={color} />
        <polygon points={`${rx},44 ${rx-4},36 ${rx+4},36`} fill={color} />
      </svg>
    </div>
  );
}

function ParallelMerge({ color, w = 580 }) {
  const cx = w / 2;
  const lx = cx - 145;
  const rx = cx + 145;
  return (
    <div style={{ position: "relative", width: w, height: 44 }}>
      <svg width={w} height={44} viewBox={`0 0 ${w} 44`} style={{ position: "absolute" }}>
        <path d={`M${lx},0 L${lx},22 L${cx},22 L${cx},44`} stroke={color} strokeWidth="1.5" fill="none" />
        <path d={`M${rx},0 L${rx},22 L${cx},22`}            stroke={color} strokeWidth="1.5" fill="none" />
        <polygon points={`${cx},44 ${cx-4},36 ${cx+4},36`} fill={color} />
      </svg>
    </div>
  );
}

const legend = [
  { color: C.entry.border,    label: "entry" },
  { color: C.scan.border,     label: "scanner / analysis" },
  { color: C.artifact.border, label: "artifact" },
  { color: C.decision.border, label: "decision" },
  { color: C.rf.border,       label: "rf engine" },
  { color: C.ai.border,       label: "ai agent" },
  { color: C.output.border,   label: "git / pr" },
];

export default function App() {
  const [active, setActive] = useState(null);
  const toggle = id => setActive(a => a === id ? null : id);
  const W = 580;

  return (
    <div style={{ minHeight: "100vh", background: "#07090f", display: "flex", flexDirection: "column", alignItems: "center", padding: "44px 20px 64px" }}>
      <style>{`@import url('https://fonts.googleapis.com/css2?family=Space+Mono:wght@400;700&display=swap');`}</style>

      <div style={{ textAlign: "center", marginBottom: "32px" }}>
        <div style={{ color: "#10b981", fontSize: "10px", letterSpacing: "0.35em", fontFamily: "'Space Mono', monospace", marginBottom: "6px" }}>PIPELINE ARCHITECTURE · v3</div>
        <div style={{ color: "#f9fafb", fontSize: "19px", fontWeight: 700, fontFamily: "'Space Mono', monospace", letterSpacing: "0.06em" }}>go-upgrade-fixer</div>
        <div style={{ color: "#374151", fontSize: "10px", fontFamily: "'Space Mono', monospace", marginTop: "4px" }}>клик на узел → описание</div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", alignItems: "center" }}>

        {/* CLI */}
        <Node node={nodes.cli} active={active} onClick={toggle} />
        <Arrow color={C.entry.accent} />

        {/* Parallel: Scan + AST Diff */}
        <SplitParallel color={C.entry.accent} w={W} />
        <div style={{ display: "flex", gap: "76px" }}>
          <Node node={nodes.scan}    active={active} onClick={toggle} />
          <Node node={nodes.astdiff} active={active} onClick={toggle} />
        </div>
        <ParallelMerge color={C.artifact.accent} w={W} />

        {/* Change Spec */}
        <Node node={nodes.changespec} active={active} onClick={toggle} />
        <Arrow color={C.decision.accent} />

        {/* Intersection */}
        <Node node={nodes.intersect} active={active} onClick={toggle} />
        <Arrow color={C.scan.accent} label="затронутые файлы + символы" />

        {/* Import Resolver */}
        <Node node={nodes.importresolver} active={active} onClick={toggle} />
        <Arrow color={C.rf.accent} />

        {/* Script Generator */}
        <Node node={nodes.scriptgen} active={active} onClick={toggle} />

        {/* SPLIT: path 1 (known) left, path 2 (unknown → AI) right */}
        <SplitDown
          w={W}
          leftColor={C.rf.accent}  leftLabel="path 1 · known patterns"
          rightColor={C.ai.accent} rightLabel="path 2 · unknown patterns"
        />

        {/* rf path 1 (left) | AI (right) */}
        <div style={{ display: "flex", gap: "76px" }}>
          <Node node={nodes.rf_known} active={active} onClick={toggle} />
          <Node node={nodes.ai}       active={active} onClick={toggle} />
        </div>

        {/* AI → rf path 2 (right column only) */}
        <div style={{ display: "flex", gap: "76px" }}>
          <div style={{ width: "252px" }} /> {/* spacer */}
          <Arrow color={C.ai.accent} label="generated rf script" />
        </div>
        <div style={{ display: "flex", gap: "76px" }}>
          <div style={{ width: "252px" }} /> {/* spacer */}
          <Node node={nodes.rf_ai} active={active} onClick={toggle} />
        </div>

        {/* Both paths merge → PR */}
        <MergeUp leftColor={C.rf.accent} rightColor={C.rf.accent} w={W} />

        {/* PR */}
        <Node node={nodes.pr} active={active} onClick={toggle} />

        {/* Legend */}
        <div style={{ marginTop: "44px", display: "flex", gap: "16px", flexWrap: "wrap", justifyContent: "center" }}>
          {legend.map(l => (
            <div key={l.label} style={{ display: "flex", alignItems: "center", gap: "5px" }}>
              <div style={{ width: "8px", height: "8px", borderRadius: "2px", background: l.color }} />
              <span style={{ color: "#4b5563", fontSize: "9px", fontFamily: "'Space Mono', monospace" }}>{l.label}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
