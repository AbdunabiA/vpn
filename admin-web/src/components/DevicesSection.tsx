import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Smartphone, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { AxiosError } from "axios";

import {
  deleteUserDevice,
  listUserDevices,
  type AdminDevice,
} from "@/api/devices";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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

// DevicesSection is a self-contained card so the user detail page can drop
// it in below the action bar without threading props. It owns its own
// TanStack Query cache entry keyed by user id.
export function DevicesSection({ userID }: { userID: string }) {
  const qc = useQueryClient();
  const [pendingDelete, setPendingDelete] = useState<AdminDevice | null>(null);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["admin", "user", userID, "devices"],
    queryFn: () => listUserDevices(userID),
    enabled: !!userID,
  });

  const deleteMutation = useMutation({
    mutationFn: (deviceRowID: string) => deleteUserDevice(userID, deviceRowID),
    onSuccess: async () => {
      await qc.invalidateQueries({
        queryKey: ["admin", "user", userID, "devices"],
      });
      toast.success("Устройство удалено");
      setPendingDelete(null);
    },
    onError: (err: unknown) => {
      const axiosErr = err as AxiosError<{ error?: string }>;
      toast.error(axiosErr.response?.data?.error ?? "Не удалось удалить");
    },
  });

  const devices = data ?? [];

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <div className="flex items-center gap-2">
          <Smartphone className="size-4 text-muted-foreground" />
          <CardTitle className="text-base">Устройства</CardTitle>
        </div>
        <span className="text-xs text-muted-foreground">
          {isLoading ? "…" : `привязано: ${devices.length}`}
        </span>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : isError ? (
          <div className="text-sm text-destructive">
            Не удалось загрузить устройства: {(error as Error).message}
          </div>
        ) : devices.length === 0 ? (
          <div className="rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
            У этого пользователя пока нет привязанных устройств.
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[120px]">ID</TableHead>
                <TableHead>Устройство</TableHead>
                <TableHead className="w-[120px]">Платформа</TableHead>
                <TableHead className="w-[180px]">Последняя активность</TableHead>
                <TableHead className="w-[60px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {devices.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {shortId(d.id)}
                  </TableCell>
                  <TableCell>
                    <div className="text-sm font-medium">
                      {d.model || "Неизвестно"}
                    </div>
                    <div
                      className="max-w-[260px] truncate font-mono text-xs text-muted-foreground"
                      title={d.device_id}
                    >
                      {d.device_id}
                    </div>
                  </TableCell>
                  <TableCell className="capitalize text-muted-foreground">
                    {d.platform || "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(d.last_seen_at)}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setPendingDelete(d)}
                      disabled={deleteMutation.isPending}
                      aria-label="Удалить устройство"
                    >
                      <Trash2 className="size-4 text-destructive" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>

      <ConfirmRemoveDialog
        device={pendingDelete}
        busy={deleteMutation.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() =>
          pendingDelete && deleteMutation.mutate(pendingDelete.id)
        }
      />
    </Card>
  );
}

function ConfirmRemoveDialog({
  device,
  busy,
  onCancel,
  onConfirm,
}: {
  device: AdminDevice | null;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={!!device} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Удалить устройство?</DialogTitle>
          <DialogDescription>
            Освободится слот устройства в тарифе пользователя. Если
            приложение всё ещё установлено, устройство автоматически
            привяжется заново при следующем гостевом входе — действие
            полностью надёжно только для потерянных или скомпрометированных
            устройств.
          </DialogDescription>
        </DialogHeader>
        {device && (
          <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
            <div className="font-medium">{device.model || "Неизвестно"}</div>
            <div className="font-mono text-xs text-muted-foreground">
              {device.device_id}
            </div>
          </div>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            type="button"
            onClick={onCancel}
            disabled={busy}
          >
            Отмена
          </Button>
          <Button
            variant="destructive"
            size="sm"
            type="button"
            onClick={onConfirm}
            disabled={busy}
          >
            Удалить
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
