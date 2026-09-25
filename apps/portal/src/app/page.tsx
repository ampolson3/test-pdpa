// Data-subject / guest portal (docs/architecture/code-structure.md: /preferences, /c/[id],
// /request, /request/status, /report-incident, /guest/...). No feature routed here yet — those
// land with CON/DSAR/BRE (P1) and the assessment/vendor guest flows (P2).
export default function PortalHome() {
  return (
    <main className="flex min-h-screen items-center justify-center">
      <p className="text-slate-500">PDPA Platform — Portal (ยังไม่มีหน้าที่ใช้งานได้ในเวอร์ชันนี้)</p>
    </main>
  );
}
