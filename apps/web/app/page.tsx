import { ApiStatus } from "../features/health/api-status";

// Foundation shell. The login, bootstrap and workflow screens belong to
// milestones M13 and M24 and need authentication (M4) first.
export default function HomePage() {
  return (
    <main>
      <h1>MORN1NGST4R</h1>
      <p>Evidence-driven security assessment, with human-verified findings.</p>
      <ApiStatus />
    </main>
  );
}
