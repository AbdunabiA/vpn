import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  Calendar,
  CheckCircle2,
  Copy,
} from "lucide-react";
import { toast } from "sonner";
import { AxiosError } from "axios";

import {
  getUser,
  updateUser,
  type AdminUser,
  type UpdateUserInput,
} from "@/api/users";
import { Button } from "@/components/ui/button";
import { ConnectionsSection } from "@/components/ConnectionsSection";
import { DevicesSection } from "@/components/DevicesSection";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import {
  roleBadgeClass,
  tierBadgeClass,
  tierDeviceLimit,
  tierLabel,
  type Tier,
} from "@/lib/tier";

// Days-to-extend presets shown on the Upgrade action. 30/90/365 covers the
// three purchase tiers we document on the landing page; the custom dialog
// handles everything else.
const PRESET_DAYS: { label: string; days: number }[] = [
  { label: "+30 дней", days: 30 },
  { label: "+90 дней", days: 90 },
  { label: "+365 дней", days: 365 },
];

// Clamp extend_days client-side. Server has no cap (flagged in architect
// review) but an admin fat-fingering a year-count into the custom dialog
// would push the expiry out to the next century. 3650 = 10 years.
const MAX_EXTEND_DAYS = 3650;

export function UserDetail() {
  const { id = "" } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const qc = useQueryClient();

  const {
    data: user,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["admin", "user", id],
    queryFn: () => getUser(id),
    enabled: !!id,
  });

  const mutation = useMutation({
    mutationFn: (input: UpdateUserInput) => updateUser(id, input),
    onSuccess: async (_data, variables) => {
      // Re-fetch the detail view and invalidate the list so the updated
      // tier/expiration surfaces immediately in both places.
      await qc.invalidateQueries({ queryKey: ["admin", "user", id] });
      await qc.invalidateQueries({ queryKey: ["admin", "users"] });
      await qc.invalidateQueries({ queryKey: ["admin", "stats"] });
      toast.success(describeUpdate(variables));
    },
    onError: (err: unknown) => {
      const axiosErr = err as AxiosError<{ error?: string }>;
      toast.error(axiosErr.response?.data?.error ?? "Не удалось обновить");
    },
  });

  const [customOpen, setCustomOpen] = useState(false);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-1/3" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  if (isError || !user) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" size="sm" onClick={() => navigate("/users")}>
          <ArrowLeft className="size-4" />
          К списку пользователей
        </Button>
        <Card className="border-destructive/40 bg-destructive/10">
          <CardContent className="p-4 text-sm text-destructive">
            Не удалось загрузить пользователя:{" "}
            {(error as Error)?.message ?? "не найден"}
          </CardContent>
        </Card>
      </div>
    );
  }

  async function copyId() {
    try {
      await navigator.clipboard.writeText(user!.id);
      toast.success("ID скопирован");
    } catch {
      toast.error("Не удалось скопировать");
    }
  }

  function applyUpgrade(tier: Exclude<Tier, "free">, days: number) {
    mutation.mutate({ subscription_tier: tier, extend_days: days });
  }

  function applyDowngrade() {
    // Downgrade to free also clears any existing expiration so the UI
    // doesn't show a stale "expires in 60 days" on a free account.
    mutation.mutate({ subscription_tier: "free", subscription_expires_at: "" });
  }

  const busy = mutation.isPending;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => navigate("/users")}
          className="-ml-2"
        >
          <ArrowLeft className="size-4" />
          К списку пользователей
        </Button>
      </div>

      {/* Profile card ----------------------------------------------------- */}
      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="space-y-2">
              <CardTitle className="text-xl">
                {user.full_name || "Без имени"}
              </CardTitle>
              <div className="flex items-center gap-2 text-xs font-mono text-muted-foreground">
                <span>{user.id}</span>
                <button
                  type="button"
                  onClick={copyId}
                  className="rounded p-1 hover:bg-accent"
                  aria-label="Скопировать ID пользователя"
                >
                  <Copy className="size-3.5" />
                </button>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <span
                className={cn(
                  "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium",
                  tierBadgeClass(user.subscription_tier),
                )}
              >
                {tierLabel(user.subscription_tier)}
              </span>
              <span
                className={cn(
                  "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium capitalize",
                  roleBadgeClass(user.role),
                )}
              >
                {user.role}
              </span>
            </div>
          </div>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Stat
            label="Лимит устройств"
            value={`${tierDeviceLimit(user.subscription_tier)} шт.`}
          />
          <Stat
            label="Истекает"
            value={formatDate(user.subscription_expires_at)}
            highlight={!!user.subscription_expires_at}
          />
          <Stat label="Создан" value={formatDate(user.created_at)} />
          <Stat label="Роль" value={user.role} />
        </CardContent>
      </Card>

      {/* Telegram recovery binding (ADR-006). Rendered as a small info
          card so admins can see whether a user has a recovery channel
          and when it was established — useful when triaging "lost my
          phone" support requests. */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Восстановление через Telegram</CardTitle>
        </CardHeader>
        <CardContent className="text-sm">
          {user.telegram_user_id ? (
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="inline-flex items-center rounded-md bg-sky-500/10 px-2 py-0.5 text-xs font-medium text-sky-300 ring-1 ring-inset ring-sky-500/30">
                  Привязан
                </span>
                {/* Prefer @username when present (public, stable,
                    recognisable) over first_name. Fall back to just
                    the tg_id for pre-016 linked rows. */}
                {user.telegram_username ? (
                  <span className="text-sm">
                    @{user.telegram_username}
                  </span>
                ) : user.telegram_first_name ? (
                  <span className="text-sm">{user.telegram_first_name}</span>
                ) : null}
                <span className="font-mono text-xs text-muted-foreground">
                  tg_id {user.telegram_user_id}
                </span>
              </div>
              <div className="text-xs text-muted-foreground">
                с {formatDate(user.telegram_linked_at)}
              </div>
            </div>
          ) : (
            <div className="text-muted-foreground">
              Не привязан. Этот пользователь не сможет восстановить аккаунт
              при смене устройства.
            </div>
          )}
        </CardContent>
      </Card>

      {/* Action bar ------------------------------------------------------- */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Действия с подпиской</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <TierActionMenu
            tier="premium"
            label="Активировать Premium"
            busy={busy}
            onSelect={(days) => applyUpgrade("premium", days)}
            currentTier={user.subscription_tier}
          />
          <TierActionMenu
            tier="ultimate"
            label="Активировать Ultimate"
            busy={busy}
            onSelect={(days) => applyUpgrade("ultimate", days)}
            currentTier={user.subscription_tier}
          />
          <Button
            variant="outline"
            size="sm"
            disabled={busy || user.subscription_tier === "free"}
            onClick={applyDowngrade}
          >
            <ArrowDown className="size-4" />
            Понизить до бесплатного
          </Button>
          <Separator orientation="vertical" className="mx-1 h-8" />
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => setCustomOpen(true)}
          >
            <Calendar className="size-4" />
            Своя дата окончания…
          </Button>
        </CardContent>
      </Card>

      <DevicesSection userID={user.id} />

      <ConnectionsSection userID={user.id} />

      <CustomExpirationDialog
        open={customOpen}
        onOpenChange={setCustomOpen}
        user={user}
        busy={busy}
        onApply={(iso) => {
          mutation.mutate({
            // Always set the tier alongside the expiration so setting a
            // date on a free user implicitly upgrades them. Leaving tier
            // as-is would be surprising: "why does my custom date not
            // work for free users?"
            subscription_tier:
              user.subscription_tier === "free"
                ? "premium"
                : user.subscription_tier,
            subscription_expires_at: iso,
          });
          setCustomOpen(false);
        }}
      />
    </div>
  );
}

function Stat({
  label,
  value,
  highlight,
}: {
  label: string;
  value: string;
  highlight?: boolean;
}) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      <div
        className={cn(
          "mt-1 text-sm capitalize",
          highlight ? "font-medium text-foreground" : "",
        )}
      >
        {value}
      </div>
    </div>
  );
}

function TierActionMenu({
  tier,
  label,
  busy,
  currentTier,
  onSelect,
}: {
  tier: Exclude<Tier, "free">;
  label: string;
  busy: boolean;
  currentTier: Tier;
  onSelect: (days: number) => void;
}) {
  const isCurrent = currentTier === tier;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" disabled={busy}>
          {isCurrent ? (
            <CheckCircle2 className="size-4 text-emerald-400" />
          ) : (
            <ArrowUp className="size-4" />
          )}
          {label}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuLabel>
          {isCurrent
            ? `Продлить ${tierLabel(tier)}`
            : `Активировать ${tierLabel(tier)}`}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {PRESET_DAYS.map((p) => (
          <DropdownMenuItem key={p.days} onSelect={() => onSelect(p.days)}>
            {p.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function CustomExpirationDialog({
  open,
  onOpenChange,
  user,
  busy,
  onApply,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: AdminUser;
  busy: boolean;
  onApply: (isoOrEmpty: string) => void;
}) {
  // Seed the date picker with whatever the user currently has set, or 30
  // days from today if they have no expiration yet. Formatted as
  // YYYY-MM-DD (UTC) for <input type="date">.
  const defaultDate = (() => {
    if (user.subscription_expires_at) {
      return toDateInputValue(new Date(user.subscription_expires_at));
    }
    // setUTCDate mirrors toDateInputValue's UTC handling so the seed
    // date is "30 days from today in UTC", not "30 days from today in
    // whatever timezone the admin happens to be in".
    const d = new Date();
    d.setUTCDate(d.getUTCDate() + 30);
    return toDateInputValue(d);
  })();

  const [date, setDate] = useState(defaultDate);
  const [error, setError] = useState<string | null>(null);

  function handleApply() {
    if (!date) {
      setError("Выберите дату");
      return;
    }
    const parsed = new Date(`${date}T23:59:59.000Z`);
    if (Number.isNaN(parsed.getTime())) {
      setError("Некорректная дата");
      return;
    }
    const deltaDays = Math.ceil((parsed.getTime() - Date.now()) / 86_400_000);
    if (deltaDays > MAX_EXTEND_DAYS) {
      setError(`Максимальное продление — ${MAX_EXTEND_DAYS / 365} лет`);
      return;
    }
    setError(null);
    onApply(parsed.toISOString());
  }

  function handleClear() {
    // Empty string on subscription_expires_at clears the backend column.
    onApply("");
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Своя дата окончания</DialogTitle>
          <DialogDescription>
            Выберите точный день, когда подписка должна истечь (UTC).
            Если очистить дату, срок действия будет снят полностью.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="expiration-date">Истекает</Label>
          <Input
            id="expiration-date"
            type="date"
            value={date}
            onChange={(e) => {
              setDate(e.target.value);
              setError(null);
            }}
            disabled={busy}
          />
          {error && (
            <div className="text-xs text-destructive">{error}</div>
          )}
        </div>
        <DialogFooter>
          <Button
            variant="ghost"
            size="sm"
            type="button"
            onClick={handleClear}
            disabled={busy}
          >
            Снять срок
          </Button>
          <div className="flex-1" />
          <Button
            variant="outline"
            size="sm"
            type="button"
            onClick={() => onOpenChange(false)}
            disabled={busy}
          >
            Отмена
          </Button>
          <Button size="sm" type="button" onClick={handleApply} disabled={busy}>
            Применить
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// toDateInputValue renders a Date as the YYYY-MM-DD string an
// <input type="date"> expects. Uses the UTC getters so the value
// round-trips cleanly through the dialog: the dialog label says
// "UTC", the picker reads UTC, the apply handler reconstructs the
// string as a UTC ISO timestamp, and an admin in a non-UTC timezone
// no longer sees their dates drift a day on every edit.
function toDateInputValue(d: Date): string {
  const yyyy = d.getUTCFullYear();
  const mm = String(d.getUTCMonth() + 1).padStart(2, "0");
  const dd = String(d.getUTCDate()).padStart(2, "0");
  return `${yyyy}-${mm}-${dd}`;
}

// describeUpdate renders a human-readable toast message from the mutation
// input so the admin gets an unambiguous confirmation of what they just
// did ("Продлён Ultimate +90 дней"), not a generic "обновлено".
function describeUpdate(input: UpdateUserInput): string {
  if (input.subscription_tier === "free") {
    return "Понижен до бесплатного";
  }
  if (input.subscription_tier && input.extend_days) {
    return `Активирован ${tierLabel(input.subscription_tier)} +${input.extend_days} дней`;
  }
  if (input.subscription_expires_at === "") {
    return "Срок действия снят";
  }
  if (input.subscription_expires_at) {
    return `Дата окончания: ${new Date(input.subscription_expires_at).toLocaleDateString("ru-RU")}`;
  }
  return "Пользователь обновлён";
}
