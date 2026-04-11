import { useState } from "react";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight } from "lucide-react";

import { listAuditEntries, type AuditEntry } from "@/api/audit";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatDate, shortId } from "@/lib/format";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 50;

// Colors for the action column. Purely cosmetic — the filter is the
// literal action name from the backend, so any new action added later
// will fall through to the default gray until it's explicitly styled.
function actionClass(action: string): string {
  if (action.startsWith("delete_")) {
    return "bg-rose-500/10 text-rose-300 ring-1 ring-inset ring-rose-500/30";
  }
  if (action.startsWith("update_") || action.startsWith("create_")) {
    return "bg-sky-500/10 text-sky-300 ring-1 ring-inset ring-sky-500/30";
  }
  if (action === "change_password") {
    return "bg-amber-500/10 text-amber-300 ring-1 ring-inset ring-amber-500/30";
  }
  return "bg-muted text-muted-foreground ring-1 ring-inset ring-border";
}

export function Activity() {
  const [page, setPage] = useState(1);
  const { data, isLoading, isError, error, isFetching } = useQuery({
    queryKey: ["admin", "audit-log", page],
    queryFn: () => listAuditEntries(page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  });

  const entries = data?.entries ?? [];
  const total = data?.pagination.total ?? 0;
  const totalPages = data?.pagination.total_pages ?? 0;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">Журнал</h1>
        <p className="text-sm text-muted-foreground">
          Сюда записывается каждое изменяющее действие администратора.
          Только для чтения; записи не удаляются после создания.
        </p>
      </div>

      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <div>
          {isLoading ? "Загрузка…" : `всего записей: ${total}`}
        </div>
        {isFetching && !isLoading && <div>Обновление…</div>}
      </div>

      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[180px]">Когда</TableHead>
              <TableHead className="w-[160px]">Действие</TableHead>
              <TableHead className="w-[120px]">Админ</TableHead>
              <TableHead className="w-[120px]">Объект</TableHead>
              <TableHead>Детали</TableHead>
              <TableHead className="w-[140px]">IP</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 6 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell colSpan={6}>
                    <Skeleton className="h-5 w-full" />
                  </TableCell>
                </TableRow>
              ))
            ) : entries.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="text-center text-sm text-muted-foreground"
                >
                  Записей пока нет. Изменения фиксируются здесь автоматически.
                </TableCell>
              </TableRow>
            ) : (
              entries.map((e) => <AuditRow key={e.id} entry={e} />)
            )}
          </TableBody>
        </Table>
      </div>

      {isError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
          Не удалось загрузить журнал: {(error as Error).message}
        </div>
      )}

      {!isLoading && total > PAGE_SIZE && (
        <div className="flex items-center justify-between text-sm">
          <div className="text-muted-foreground">
            Страница {page} из {totalPages}
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
            >
              <ChevronLeft className="size-4" />
              Назад
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              Вперёд
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function AuditRow({ entry }: { entry: AuditEntry }) {
  return (
    <TableRow>
      <TableCell className="text-xs text-muted-foreground">
        {formatDate(entry.created_at)}
      </TableCell>
      <TableCell>
        <span
          className={cn(
            "inline-flex items-center rounded-md px-2 py-0.5 font-mono text-xs",
            actionClass(entry.action),
          )}
        >
          {entry.action}
        </span>
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {shortId(entry.admin_id)}
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {entry.target_id ? shortId(entry.target_id) : "—"}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        <DetailsCell details={entry.details} />
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {entry.ip || "—"}
      </TableCell>
    </TableRow>
  );
}

function DetailsCell({ details }: { details: AuditEntry["details"] }) {
  if (!details) return <span>—</span>;
  // Compact one-line key=value rendering; the most useful bits are
  // method + path + query. Non-primitive values get JSON.stringify so
  // objects render as {"k":"v"} instead of "[object Object]".
  const entries = Object.entries(details)
    .filter(([k]) => k !== "method" && k !== "path")
    .map(([k, v]) => {
      const s =
        typeof v === "string" || typeof v === "number" || typeof v === "boolean"
          ? String(v)
          : JSON.stringify(v);
      return `${k}=${s}`;
    })
    .join(" ");
  return (
    <div className="max-w-[360px] truncate" title={JSON.stringify(details)}>
      <span className="text-foreground/70">
        {String(details.method ?? "")}{" "}
        <span className="font-mono">{String(details.path ?? "")}</span>
      </span>
      {entries && <span className="ml-2">{entries}</span>}
    </div>
  );
}
