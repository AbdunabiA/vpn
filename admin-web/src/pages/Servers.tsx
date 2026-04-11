import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Power, PowerOff } from "lucide-react";
import { toast } from "sonner";
import { AxiosError } from "axios";

import {
  deleteServer,
  listServers,
  updateServer,
  type AdminServer,
} from "@/api/servers";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { formatDate } from "@/lib/format";

export function Servers() {
  const qc = useQueryClient();
  const [pendingDelete, setPendingDelete] = useState<AdminServer | null>(null);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["admin", "servers"],
    queryFn: listServers,
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, isActive }: { id: string; isActive: boolean }) =>
      updateServer(id, { is_active: isActive }),
    onSuccess: async (_data, vars) => {
      await qc.invalidateQueries({ queryKey: ["admin", "servers"] });
      await qc.invalidateQueries({ queryKey: ["admin", "stats"] });
      toast.success(
        vars.isActive ? "Сервер активирован" : "Сервер деактивирован",
      );
    },
    onError: (err: unknown) => {
      const axiosErr = err as AxiosError<{ error?: string }>;
      toast.error(axiosErr.response?.data?.error ?? "Не удалось обновить");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteServer(id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["admin", "servers"] });
      await qc.invalidateQueries({ queryKey: ["admin", "stats"] });
      toast.success("Сервер деактивирован (мягкое удаление)");
      setPendingDelete(null);
    },
    onError: (err: unknown) => {
      const axiosErr = err as AxiosError<{ error?: string }>;
      toast.error(axiosErr.response?.data?.error ?? "Не удалось удалить");
    },
  });

  const servers = data ?? [];
  const busy = toggleMutation.isPending || deleteMutation.isPending;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Серверы</h1>
        <p className="text-sm text-muted-foreground">
          Список VPN-серверов. Переключайте активность, чтобы временно
          вывести сервер из ротации без потери конфигурации.
        </p>
      </div>

      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Хост</TableHead>
              <TableHead className="w-[180px]">Локация</TableHead>
              <TableHead className="w-[140px]">Протокол</TableHead>
              <TableHead className="w-[100px]">Нагрузка</TableHead>
              <TableHead className="w-[100px]">Статус</TableHead>
              <TableHead className="w-[180px]">Создан</TableHead>
              <TableHead className="w-[120px]" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell colSpan={7}>
                    <Skeleton className="h-5 w-full" />
                  </TableCell>
                </TableRow>
              ))
            ) : servers.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="text-center text-sm text-muted-foreground"
                >
                  Нет настроенных серверов.
                </TableCell>
              </TableRow>
            ) : (
              servers.map((s) => (
                <ServerRow
                  key={s.id}
                  server={s}
                  busy={busy}
                  onToggle={() =>
                    toggleMutation.mutate({ id: s.id, isActive: !s.is_active })
                  }
                  onDelete={() => setPendingDelete(s)}
                />
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {isError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
          Не удалось загрузить серверы: {(error as Error).message}
        </div>
      )}

      <Dialog
        open={!!pendingDelete}
        onOpenChange={(open) => !open && setPendingDelete(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Деактивировать сервер?</DialogTitle>
            <DialogDescription>
              Это мягкое удаление — строка остаётся в базе с
              is_active=false. Вы можете снова включить сервер в любой
              момент на этой странице. Подключённые клиенты отвалятся
              после того, как текущая нагрузка сойдёт на нет.
            </DialogDescription>
          </DialogHeader>
          {pendingDelete && (
            <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
              <div className="font-medium">{pendingDelete.hostname}</div>
              <div className="font-mono text-xs text-muted-foreground">
                {pendingDelete.ip_address} · {pendingDelete.city},{" "}
                {pendingDelete.country}
              </div>
            </div>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              size="sm"
              type="button"
              onClick={() => setPendingDelete(null)}
              disabled={busy}
            >
              Отмена
            </Button>
            <Button
              variant="destructive"
              size="sm"
              type="button"
              disabled={busy}
              onClick={() =>
                pendingDelete && deleteMutation.mutate(pendingDelete.id)
              }
            >
              Деактивировать
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function ServerRow({
  server,
  busy,
  onToggle,
  onDelete,
}: {
  server: AdminServer;
  busy: boolean;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const load = server.load_percent ?? 0;
  const hot = load >= 80;

  return (
    <TableRow className={cn(!server.is_active && "opacity-60")}>
      <TableCell>
        <div className="font-medium">{server.hostname}</div>
        <div className="font-mono text-xs text-muted-foreground">
          {server.ip_address}
        </div>
      </TableCell>
      <TableCell>
        <div className="text-sm">
          {server.city}, {server.country}
        </div>
        <div className="text-xs uppercase text-muted-foreground">
          {server.region}
        </div>
      </TableCell>
      <TableCell>
        <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-mono ring-1 ring-inset ring-border">
          {server.protocol}
        </span>
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
            <div
              className={cn(
                "h-full rounded-full transition-all",
                hot ? "bg-amber-400" : "bg-emerald-400",
              )}
              style={{ width: `${Math.min(100, Math.max(0, load))}%` }}
            />
          </div>
          <span
            className={cn(
              "text-xs tabular-nums",
              hot ? "text-amber-300" : "text-muted-foreground",
            )}
          >
            {load}%
          </span>
        </div>
      </TableCell>
      <TableCell>
        <span
          className={cn(
            "inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium",
            server.is_active
              ? "bg-emerald-500/10 text-emerald-300 ring-1 ring-inset ring-emerald-500/30"
              : "bg-muted text-muted-foreground ring-1 ring-inset ring-border",
          )}
        >
          {server.is_active ? "Активен" : "Неактивен"}
        </span>
      </TableCell>
      <TableCell className="text-muted-foreground">
        {formatDate(server.created_at)}
      </TableCell>
      <TableCell className="text-right">
        <div className="flex items-center justify-end gap-1">
          <Button
            variant="ghost"
            size="icon"
            onClick={onToggle}
            disabled={busy}
            aria-label={server.is_active ? "Деактивировать" : "Активировать"}
            title={server.is_active ? "Деактивировать" : "Активировать"}
          >
            {server.is_active ? (
              <PowerOff className="size-4" />
            ) : (
              <Power className="size-4 text-emerald-400" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={onDelete}
            disabled={busy || !server.is_active}
            className="text-destructive hover:text-destructive"
          >
            Удалить
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}
