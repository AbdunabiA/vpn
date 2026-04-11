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
  { label: "+30 days", days: 30 },
  { label: "+90 days", days: 90 },
  { label: "+365 days", days: 365 },
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
      toast.error(axiosErr.response?.data?.error ?? "Update failed");
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
          Back to users
        </Button>
        <Card className="border-destructive/40 bg-destructive/10">
          <CardContent className="p-4 text-sm text-destructive">
            Failed to load user: {(error as Error)?.message ?? "not found"}
          </CardContent>
        </Card>
      </div>
    );
  }

  async function copyId() {
    try {
      await navigator.clipboard.writeText(user!.id);
      toast.success("User ID copied");
    } catch {
      toast.error("Copy failed");
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
          Back to users
        </Button>
      </div>

      {/* Profile card ----------------------------------------------------- */}
      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="space-y-2">
              <CardTitle className="text-xl">
                {user.full_name || "Unnamed user"}
              </CardTitle>
              <div className="flex items-center gap-2 text-xs font-mono text-muted-foreground">
                <span>{user.id}</span>
                <button
                  type="button"
                  onClick={copyId}
                  className="rounded p-1 hover:bg-accent"
                  aria-label="Copy user ID"
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
          <Stat label="Device limit" value={`${tierDeviceLimit(user.subscription_tier)} device(s)`} />
          <Stat
            label="Expires"
            value={formatDate(user.subscription_expires_at)}
            highlight={!!user.subscription_expires_at}
          />
          <Stat label="Created" value={formatDate(user.created_at)} />
          <Stat label="Role" value={user.role} />
        </CardContent>
      </Card>

      {/* Action bar ------------------------------------------------------- */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Subscription actions</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <TierActionMenu
            tier="premium"
            label="Upgrade to Premium"
            busy={busy}
            onSelect={(days) => applyUpgrade("premium", days)}
            currentTier={user.subscription_tier}
          />
          <TierActionMenu
            tier="ultimate"
            label="Upgrade to Ultimate"
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
            Downgrade to Free
          </Button>
          <Separator orientation="vertical" className="mx-1 h-8" />
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => setCustomOpen(true)}
          >
            <Calendar className="size-4" />
            Custom expiration…
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
          {isCurrent ? `Extend ${tierLabel(tier)}` : `Activate ${tierLabel(tier)}`}
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
  // YYYY-MM-DD for <input type="date">.
  const defaultDate = (() => {
    if (user.subscription_expires_at) {
      return toDateInputValue(new Date(user.subscription_expires_at));
    }
    const d = new Date();
    d.setDate(d.getDate() + 30);
    return toDateInputValue(d);
  })();

  const [date, setDate] = useState(defaultDate);
  const [error, setError] = useState<string | null>(null);

  function handleApply() {
    if (!date) {
      setError("Please pick a date");
      return;
    }
    const parsed = new Date(`${date}T23:59:59.000Z`);
    if (Number.isNaN(parsed.getTime())) {
      setError("Invalid date");
      return;
    }
    const deltaDays = Math.ceil((parsed.getTime() - Date.now()) / 86_400_000);
    if (deltaDays > MAX_EXTEND_DAYS) {
      setError(`Maximum extension is ${MAX_EXTEND_DAYS / 365} years`);
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
          <DialogTitle>Set custom expiration</DialogTitle>
          <DialogDescription>
            Pick the exact day the subscription should expire (UTC).
            Clearing the date removes the expiration entirely.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="expiration-date">Expires on</Label>
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
            Clear expiration
          </Button>
          <div className="flex-1" />
          <Button
            variant="outline"
            size="sm"
            type="button"
            onClick={() => onOpenChange(false)}
            disabled={busy}
          >
            Cancel
          </Button>
          <Button size="sm" type="button" onClick={handleApply} disabled={busy}>
            Apply
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function toDateInputValue(d: Date): string {
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  return `${yyyy}-${mm}-${dd}`;
}

// describeUpdate renders a human-readable toast message from the mutation
// input so the admin gets an unambiguous confirmation of what they just
// did ("Extended Ultimate by 90 days"), not a generic "updated".
function describeUpdate(input: UpdateUserInput): string {
  if (input.subscription_tier === "free") {
    return "Downgraded to Free";
  }
  if (input.subscription_tier && input.extend_days) {
    return `Activated ${tierLabel(input.subscription_tier)} +${input.extend_days} days`;
  }
  if (input.subscription_expires_at === "") {
    return "Expiration cleared";
  }
  if (input.subscription_expires_at) {
    return `Expiration set to ${new Date(input.subscription_expires_at).toLocaleDateString()}`;
  }
  return "User updated";
}
