import { RunCounter } from "@/components/RunCounter";

const newsletterSignupUrl = "https://drjoshcsimmons.kit.com/";

const navItems = [
  { href: "#signal", label: "Signal Path" },
  { href: "#control-surface", label: "Control Surface" },
  { href: "#install", label: "Install" },
  { href: "#broadcast", label: "Broadcast" },
] as const;

const statusCells = [
  {
    label: "providers",
    value: "Codex + Claude Code",
    note: "Same loop. Different operators.",
  },
  {
    label: "memory model",
    value: "Cold starts only",
    note: "No fragile chat residue between passes.",
  },
  {
    label: "artifact tape",
    value: "Record",
    note: "Prompt, stderr, result JSON, diff patch.",
  },
  {
    label: "stall control",
    value: "Forced pivots",
    note: "When the oscillator is spent, route elsewhere.",
  },
] as const;

const modules = [
  {
    eyebrow: "Module 01",
    title: "One concrete improvement per cycle",
    text: "ROOM keeps the loop narrow on purpose: one worthwhile improvement, one validated JSON result, one commit decision.",
    accent: "lime",
  },
  {
    eyebrow: "Module 02",
    title: "Cold-start prompts stay honest",
    text: "Each pass rebuilds context from local repo state, recent summaries, commits, and the current instruction instead of trusting chat drift.",
    accent: "coral",
  },
  {
    eyebrow: "Module 03",
    title: "Failure tape stays inspectable",
    text: "Malformed JSON, provider issues, tiny diffs, and clipped signals all leave artifacts behind so the run can be debugged after the fact.",
    accent: "cyan",
  },
  {
    eyebrow: "Module 04",
    title: "Stagnation triggers a reroute",
    text: "Duplicate instructions, churn loops, and spent subsystems get pressure-tested until ROOM rewrites the next instruction into a pivot.",
    accent: "sun",
  },
  {
    eyebrow: "Module 05",
    title: "Built like an operator tool",
    text: "The product is a local power tool, not a polite platform. It assumes git, authenticated CLIs, and someone who wants the tape.",
    accent: "magenta",
  },
  {
    eyebrow: "Module 06",
    title: "Live TUI energy, not dashboard mush",
    text: "ROOM’s visual language leans modular synth and signal trace: oscillators, resonance, overload, queue depth, pivots, and tape.",
    accent: "paper",
  },
] as const;

const sequence = [
  {
    step: "01",
    title: "Seed the room",
    text: "Initialize local state, schema, instruction tape, summaries, and run directories inside `.room/`.",
  },
  {
    step: "02",
    title: "Fire the loop",
    text: "Build fresh prompt context and drive the selected CLI headlessly with a tight JSON contract.",
  },
  {
    step: "03",
    title: "Read the meters",
    text: "Store stdout, stderr, result, metadata, and diff artifacts so every iteration can be replayed like a diagnostic trace.",
  },
  {
    step: "04",
    title: "Commit or pivot",
    text: "Keep strong changes, detect stale momentum, and force the next instruction to move when the signal path starts circling.",
  },
] as const;

const commands = [
  "curl -fsSL https://raw.githubusercontent.com/jcpsimmons/room/main/scripts/install.sh | sh",
  'room init --prompt "Make this repository materially better."',
  "room doctor",
  "room run --iterations 5",
] as const;

const tapes = [
  "prompt.txt",
  "execution.json",
  "stdout.log",
  "stderr.log",
  "result.json",
  "diff.patch",
] as const;

const meterLevels = [
  { id: "queue-depth", level: 84 },
  { id: "momentum", level: 63 },
  { id: "artifact-integrity", level: 92 },
  { id: "retry-voltage", level: 57 },
  { id: "resonance", level: 74 },
  { id: "pivot-pressure", level: 48 },
] as const;

const tickerItems = [
  "codex",
  "claude code",
  "cold-start prompts",
  "forced pivots",
  "we are all on the same universal spaceship",
  "one improvement per pass",
  "signal clipped? inspect the tape",
  "one universe",
  "one world",
  "one love?",
  ".room state stays local",
  "operator-grade loop control",
] as const;

function LinkChip({
  href,
  children,
  dark = false,
}: {
  href: string;
  children: React.ReactNode;
  dark?: boolean;
}) {
  return (
    <a
      className={dark ? "link-chip link-chip--dark" : "link-chip"}
      href={href}
      target="_blank"
      rel="noreferrer"
    >
      {children}
    </a>
  );
}

export default function HomePage() {
  return (
    <main className="page-shell">
      <header className="masthead">
        <div className="masthead__brand">
          <span className="masthead__badge">ROOM</span>
          <p className="masthead__caption">
            Repetitively Obsessively Optimize Me
          </p>
        </div>
        <nav className="masthead__nav" aria-label="Page navigation">
          {navItems.map((item) => (
            <a key={item.href} href={item.href}>
              {item.label}
            </a>
          ))}
        </nav>
      </header>

      <section className="hero-grid" id="signal">
        <div className="panel panel--hero">
          <div className="hero-copy">
            <p className="eyebrow">Voltage-controlled repo sequencer</p>
            <h1>REPETITIVELY OBSESSIVELY OPTIMIZE ME</h1>
            <p className="hero-copy__lede">
              ROOM runs a self-perpetuating (not self-hating) cold-start loop
              against your git repo. Stop micromanaging coding agents, let go
              and trust the process. Your idea sails on carefully curated
              universal background radiation to settle into a groove. It's kinda
              like raising a kid, you give some guidance but then they grow up.
            </p>
            <div className="example-spotlight">
              <p className="eyebrow">Example build</p>
              <a
                className="example-spotlight__link"
                href="https://room-signal-garden.vercel.app/"
                target="_blank"
                rel="noreferrer"
              >
                room-signal-garden.vercel.app
              </a>
              <p className="example-spotlight__note">
                Built from one prompt. Then gpt-5.3-codex-spark ran 100
                iterations without my input.
              </p>
            </div>
            <div className="hero-copy__actions">
              <a
                className="action-button"
                href="https://github.com/jcpsimmons/room"
                target="_blank"
                rel="noreferrer"
              >
                Inspect the GitHub repo
              </a>
              <a className="action-button action-button--ghost" href="#install">
                Start the sequence
              </a>
            </div>
            <div className="hero-sticker-cluster">
              <span className="hero-sticker hero-sticker--lime">
                cold starts only
              </span>
              <span className="hero-sticker hero-sticker--ink">
                forced pivots
              </span>
              <span className="hero-sticker hero-sticker--coral">
                artifact tape intact
              </span>
            </div>
          </div>
        </div>

        <div className="panel panel--scope">
          <div className="scope-display" aria-hidden="true">
            <div className="scope-display__orbit scope-display__orbit--outer" />
            <div className="scope-display__orbit scope-display__orbit--mid" />
            <div className="scope-display__orbit scope-display__orbit--inner" />
            <div className="scope-display__core">
              <span>RUN</span>
              <RunCounter />
            </div>
            <div className="scope-display__tag scope-display__tag--a">
              prompt
            </div>
            <div className="scope-display__tag scope-display__tag--b">diff</div>
            <div className="scope-display__tag scope-display__tag--c">
              commit
            </div>
            <div className="scope-display__tag scope-display__tag--d">
              pivot
            </div>
          </div>

          <div className="scope-readout">
            {statusCells.map((cell) => (
              <article key={cell.label} className="status-cell">
                <p>{cell.label}</p>
                <h2>{cell.value}</h2>
                <span>{cell.note}</span>
              </article>
            ))}
          </div>
        </div>
      </section>

      <div className="ticker">
        <div className="ticker__rail">
          {[0, 1].map((group) => (
            <div
              key={`ticker-group-${group}`}
              className="ticker__group"
              aria-hidden={group === 1}
            >
              {tickerItems.map((item) => (
                <span key={`${group}-${item}`}>{item}</span>
              ))}
            </div>
          ))}
        </div>
      </div>

      <section className="module-grid" id="control-surface">
        {modules.map((module) => (
          <article
            key={module.title}
            className={`module-card module-card--${module.accent}`}
          >
            <p className="eyebrow">{module.eyebrow}</p>
            <h2>{module.title}</h2>
            <p>{module.text}</p>
          </article>
        ))}
      </section>

      <section className="instrument-grid">
        <article className="panel panel--sequence">
          <div className="section-heading">
            <p className="eyebrow">Signal choreography</p>
            <h2>Four disciplined moves. No vague orchestration theater.</h2>
          </div>
          <div className="sequence-grid">
            {sequence.map((item) => (
              <article key={item.step} className="sequence-step">
                <span className="sequence-step__index">{item.step}</span>
                <h3>{item.title}</h3>
                <p>{item.text}</p>
              </article>
            ))}
          </div>
        </article>

        <article className="panel panel--meters">
          <div className="section-heading">
            <p className="eyebrow">Live meters</p>
            <h2>Progress should look and sound like a machine under load.</h2>
          </div>
          <div className="meter-bank" aria-hidden="true">
            {meterLevels.map((meter) => (
              <span
                key={meter.id}
                className="meter-bank__bar"
                style={{ ["--level" as string]: `${meter.level}%` }}
              />
            ))}
          </div>
          <div className="trace-grid" aria-hidden="true">
            <span className="trace-grid__pulse trace-grid__pulse--a" />
            <span className="trace-grid__pulse trace-grid__pulse--b" />
            <span className="trace-grid__pulse trace-grid__pulse--c" />
          </div>
        </article>
      </section>

      <section className="install-grid" id="install">
        <article className="panel panel--install">
          <div className="section-heading">
            <p className="eyebrow">Quick route</p>
            <h2>Install it, seed it, pressure the repo, and keep the tape.</h2>
          </div>
          <div className="command-stack">
            {commands.map((command) => (
              <code key={command} className="command-line">
                {command}
              </code>
            ))}
          </div>
          <div className="panel-links">
            <LinkChip href="https://github.com/jcpsimmons/room/releases">
              Releases
            </LinkChip>
          </div>
        </article>

        <article className="panel panel--artifact">
          <div className="section-heading">
            <p className="eyebrow">Artifact tape</p>
            <h2>Every run leaves a readable trail.</h2>
          </div>
          <div className="tape-list">
            {tapes.map((item) => (
              <div key={item} className="tape-list__row">
                <span>{item}</span>
                <span>captured</span>
              </div>
            ))}
          </div>
          <p className="panel-note">
            ROOM keeps `.room/` out of its own dirty checks and commits so the
            operator state stays local and the repository stays sane.
          </p>
        </article>
      </section>

      <section className="broadcast-grid" id="broadcast">
        <article className="panel panel--broadcast">
          <div className="section-heading">
            <p className="eyebrow">External channels</p>
            <h2>Keep the signal moving outside the terminal.</h2>
          </div>
          <div className="link-chip-grid">
            <LinkChip href="https://github.com/jcpsimmons/room" dark>
              GitHub repo
            </LinkChip>
            <LinkChip href="https://www.youtube.com/@drjoshcsimmons" dark>
              YouTube channel
            </LinkChip>
            <LinkChip href={newsletterSignupUrl} dark>
              Newsletter sign up
            </LinkChip>
          </div>
          <p className="panel-note">
            The voice here is the same as the product: sharp, inspectable, and
            allergic to generic SaaS beige.
          </p>
        </article>

        <article className="panel panel--newsletter">
          <div className="section-heading">
            <p className="eyebrow">Newsletter sign up</p>
            <h2>Take the external page for the newsletter.</h2>
          </div>
          <p className="panel-note">
            The signup lives off-site. Follow the link for the external form and
            join the list there.
          </p>
          <div className="panel-links">
            <LinkChip href={newsletterSignupUrl} dark>
              Newsletter sign up
            </LinkChip>
          </div>
        </article>
      </section>
    </main>
  );
}
