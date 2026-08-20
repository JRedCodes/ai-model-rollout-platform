import { architectureDiagram, flowSteps, services } from "./content/architecture.ts";
import { navigate } from "./router.ts";

const REPO_URL = "https://github.com/JRedCodes/ai-model-rollout-platform";

export function AboutPage() {
  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans">
      <header className="border-b border-slate-800 px-6 py-4 flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="w-2 h-2 rounded-full bg-emerald-500" />
          <span className="text-sm font-semibold tracking-widest uppercase text-slate-400">
            Rollout Platform
          </span>
        </div>
        <button
          type="button"
          onClick={() => navigate("/")}
          className="text-xs text-slate-500 hover:text-slate-300 underline underline-offset-2"
        >
          Back
        </button>
      </header>

      <main className="max-w-3xl mx-auto p-6 space-y-8">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">How this works</h1>
          <p className="text-sm text-slate-400 mt-2">
            A distributed system that safely rolls out a new AI model version by shifting traffic to
            it incrementally, watching error rates and latency the whole way, and rolling back
            automatically if something looks wrong.
          </p>
        </div>

        <div className="rounded-lg border border-amber-900/50 bg-amber-950/20 p-4 text-sm text-amber-300">
          <strong className="font-semibold">This is a simulated demo environment.</strong> No real
          model inference happens here -- the Model Service fabricates responses with a configurable
          failure rate and latency range, so a rollout's health can be steered on demand to show
          every code path (advance, hold, resume, rollback) without needing a real model or real
          production traffic.
        </div>

        <section>
          <h2 className="text-xs font-semibold tracking-widest uppercase text-slate-500 mb-3">
            Architecture
          </h2>
          <div className="rounded-lg border border-slate-800 bg-slate-900 p-4 overflow-x-auto">
            <pre className="text-[11px] leading-tight text-slate-400 font-mono whitespace-pre">
              {architectureDiagram}
            </pre>
          </div>
        </section>

        <section className="space-y-4">
          <h2 className="text-xs font-semibold tracking-widest uppercase text-slate-500">
            Data & control flow
          </h2>
          {flowSteps.map((step) => (
            <div key={step.title} className="rounded-lg border border-slate-800 bg-slate-900 p-4">
              <p className="text-sm font-medium text-slate-200">{step.title}</p>
              <p className="text-sm text-slate-400 mt-1">{step.description}</p>
            </div>
          ))}
        </section>

        <section className="space-y-3">
          <h2 className="text-xs font-semibold tracking-widest uppercase text-slate-500">
            Components
          </h2>
          <div className="grid sm:grid-cols-2 gap-3">
            {services.map((service) => (
              <div
                key={service.name}
                className="rounded-lg border border-slate-800 bg-slate-900 p-4"
              >
                <div className="flex items-baseline justify-between gap-2">
                  <p className="text-sm font-medium text-slate-200">{service.name}</p>
                  <p className="text-xs text-slate-600 font-mono">{service.tech}</p>
                </div>
                <p className="text-xs text-slate-600 font-mono mt-0.5">{service.location}</p>
                <p className="text-sm text-slate-400 mt-2">{service.summary}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="rounded-lg border border-slate-800 bg-slate-900 p-4 space-y-2">
          <h2 className="text-xs font-semibold tracking-widest uppercase text-slate-500">
            Playing with it
          </h2>
          <ol className="text-sm text-slate-400 list-decimal list-inside space-y-1">
            <li>Sign up for an account -- this creates your own isolated tenant and a rollout.</li>
            <li>Copy the API key shown once at sign-up (or regenerate one later from Account).</li>
            <li>
              Run the stress-tester CLI with that key to generate traffic and watch the rollout
              advance, hold, or roll back live in this dashboard.
            </li>
          </ol>
          <p className="text-xs text-slate-600 pt-1">
            Full setup instructions (prerequisites, running each service locally, CLI usage) live in{" "}
            <a
              href={REPO_URL}
              target="_blank"
              rel="noreferrer"
              className="text-slate-500 hover:text-slate-300 underline underline-offset-2"
            >
              the project's README
            </a>
            . Curious how a specific piece works under the hood?{" "}
            <a
              href={`${REPO_URL}/tree/main/documents`}
              target="_blank"
              rel="noreferrer"
              className="text-slate-500 hover:text-slate-300 underline underline-offset-2"
            >
              Each service has its own deep-dive doc
            </a>
            .
          </p>
        </section>
      </main>
    </div>
  );
}
