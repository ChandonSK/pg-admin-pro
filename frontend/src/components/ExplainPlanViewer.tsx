import { AlertTriangle, Clock, GitBranch, Layers } from 'lucide-react';
import type { ExplainResult } from '../types';

interface ExplainPlanViewerProps {
  result: ExplainResult;
}

interface PgPlanNode {
  'Node Type'?: string;
  'Relation Name'?: string;
  Alias?: string;
  'Join Type'?: string;
  'Index Name'?: string;
  'Index Cond'?: string;
  Filter?: string;
  'Hash Cond'?: string;
  'Sort Key'?: string[];
  'Startup Cost'?: number;
  'Total Cost'?: number;
  'Plan Rows'?: number;
  'Actual Rows'?: number;
  'Actual Loops'?: number;
  'Actual Total Time'?: number;
  Plans?: PgPlanNode[];
}

function PgNodeCard({ node, depth = 0 }: { node: PgPlanNode; depth?: number }) {
  const nodeType = node['Node Type'] || 'Plan Node';
  const rel = node['Relation Name'] || node.Alias || '';
  const planRows = node['Plan Rows'] ?? 0;
  const actualRows = node['Actual Rows'];
  const loops = node['Actual Loops'] ?? 1;
  const actualMs = node['Actual Total Time'];
  const isSeqScanWarning = nodeType === 'Seq Scan' && (actualRows ?? planRows) >= 1000;

  return (
    <div className={depth > 0 ? 'ml-5 pl-3 border-l-2 border-indigo-500/30 mt-2' : 'mt-1'}>
      <div
        className={`rounded-xl border p-3 text-xs space-y-1.5 ${
          isSeqScanWarning
            ? 'border-amber-500/40 bg-amber-500/5'
            : 'border-slate-200 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-950/60'
        }`}
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-semibold text-indigo-600 dark:text-indigo-400">
              {nodeType}
              {node['Join Type'] ? ` (${node['Join Type']})` : ''}
            </span>
            {rel && (
              <span className="px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono text-[11px]">
                {rel}
              </span>
            )}
            {node['Index Name'] && (
              <span className="px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 font-mono text-[10px]">
                using {node['Index Name']}
              </span>
            )}
            {isSeqScanWarning && (
              <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-600 dark:text-amber-300 text-[10px] font-semibold">
                <AlertTriangle className="w-2.5 h-2.5" /> Large Seq Scan
              </span>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-3 font-mono text-[11px] text-slate-500">
            <span>
              Cost: {node['Startup Cost'] ?? 0}..{node['Total Cost'] ?? 0}
            </span>
            <span>Est: {planRows.toLocaleString()} rows</span>
            {actualRows !== undefined && (
              <span className="text-slate-900 dark:text-slate-100 font-semibold">
                Actual: {actualRows.toLocaleString()} &times; {loops}
              </span>
            )}
            {actualMs !== undefined && (
              <span className="text-indigo-500 font-semibold">
                {(actualMs * loops).toFixed(2)} ms
              </span>
            )}
          </div>
        </div>

        {(node['Index Cond'] || node.Filter || node['Hash Cond'] || node['Sort Key']) && (
          <div className="space-y-0.5 font-mono text-[11px] text-slate-500">
            {node['Index Cond'] && <div>Index Cond: {node['Index Cond']}</div>}
            {node['Hash Cond'] && <div>Hash Cond: {node['Hash Cond']}</div>}
            {node['Sort Key'] && <div>Sort Key: {node['Sort Key'].join(', ')}</div>}
            {node.Filter && <div>Filter: {node.Filter}</div>}
          </div>
        )}
      </div>

      {node.Plans?.map((child, idx) => (
        <PgNodeCard key={idx} node={child} depth={depth + 1} />
      ))}
    </div>
  );
}

export function ExplainPlanViewer({ result }: ExplainPlanViewerProps) {
  let rootPlan: PgPlanNode | null = null;
  let planningTime: number | undefined;
  let execTime: number | undefined;

  if (result.planJson) {
    const first = Array.isArray(result.planJson) ? result.planJson[0] : result.planJson;
    if (first && typeof first === 'object') {
      const obj = first as {
        Plan?: PgPlanNode;
        'Planning Time'?: number;
        'Execution Time'?: number;
      };
      if (obj.Plan) {
        rootPlan = obj.Plan;
        planningTime = obj['Planning Time'];
        execTime = obj['Execution Time'];
      }
    }
  }

  return (
    <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/80 overflow-hidden">
      <div className="px-4 py-2.5 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 flex flex-wrap items-center justify-between gap-2 text-xs">
        <div className="flex items-center gap-2 font-semibold">
          <GitBranch className="w-3.5 h-3.5 text-indigo-500" />
          Execution Plan ({result.analyzed ? 'EXPLAIN ANALYZE — rolled back safely' : 'EXPLAIN ESTIMATE'})
        </div>
        <div className="flex items-center gap-3 font-mono text-[11px] text-slate-500">
          {planningTime !== undefined && <span>Planning: {planningTime.toFixed(2)} ms</span>}
          {execTime !== undefined && (
            <span className="text-indigo-500 font-semibold">
              Execution: {execTime.toFixed(2)} ms
            </span>
          )}
          <span className="inline-flex items-center gap-1">
            <Clock className="w-3 h-3" /> {result.durationMs}ms total
          </span>
        </div>
      </div>

      <div className="p-4 max-h-[460px] overflow-auto">
        {rootPlan ? (
          <PgNodeCard node={rootPlan} />
        ) : result.planLines && result.planLines.length > 0 ? (
          <pre className="text-xs font-mono text-slate-800 dark:text-slate-200 whitespace-pre-wrap">
            {result.planLines.join('\n')}
          </pre>
        ) : (
          <div className="flex items-center gap-2 text-xs text-slate-400">
            <Layers className="w-4 h-4" />
            <pre className="whitespace-pre-wrap font-mono">
              {JSON.stringify(result.planJson, null, 2)}
            </pre>
          </div>
        )}
      </div>
    </div>
  );
}
