import { FormEvent, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { AxiosError } from "axios";
import { KeyRound, LogOut, User } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { changeAdminPassword } from "@/api/account";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { useAuthStore } from "@/stores/authStore";

export function Settings() {
  const user = useAuthStore((s) => s.user);
  const clear = useAuthStore((s) => s.clear);
  const navigate = useNavigate();

  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: (input: { current: string; next: string }) =>
      changeAdminPassword(input.current, input.next),
    onSuccess: () => {
      toast.success("Password updated");
      setCurrent("");
      setNext("");
      setConfirm("");
    },
    onError: (err: unknown) => {
      const axiosErr = err as AxiosError<{ error?: string }>;
      toast.error(axiosErr.response?.data?.error ?? "Password change failed");
    },
  });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setLocalError(null);
    if (next.length < 8) {
      setLocalError("New password must be at least 8 characters");
      return;
    }
    if (next.length > 72) {
      setLocalError("New password must be at most 72 characters");
      return;
    }
    if (next !== confirm) {
      setLocalError("New password and confirmation do not match");
      return;
    }
    if (next === current) {
      setLocalError("New password must differ from the current one");
      return;
    }
    mutation.mutate({ current, next });
  }

  function handleLogout() {
    clear();
    navigate("/login", { replace: true });
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Manage your admin account.
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <User className="size-4 text-muted-foreground" />
            <CardTitle className="text-base">Account</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <Row label="Name" value={user?.full_name ?? "—"} />
          <Row label="User ID" value={user?.id ?? "—"} mono />
          <Row label="Role" value={user?.role ?? "—"} />
          <Row label="Tier" value={user?.subscription_tier ?? "—"} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <KeyRound className="size-4 text-muted-foreground" />
            <CardTitle className="text-base">Change password</CardTitle>
          </div>
          <CardDescription>
            Pick something long (8–72 characters). If you are still using
            the seed password from first provisioning, rotate it now. Other
            active sessions stay alive until their refresh token expires —
            this is a deliberate choice, not a bug.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4 max-w-sm" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label htmlFor="current-password">Current password</Label>
              <Input
                id="current-password"
                type="password"
                autoComplete="current-password"
                value={current}
                onChange={(e) => setCurrent(e.target.value)}
                disabled={mutation.isPending}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="new-password">New password</Label>
              <Input
                id="new-password"
                type="password"
                autoComplete="new-password"
                value={next}
                onChange={(e) => setNext(e.target.value)}
                disabled={mutation.isPending}
                required
                minLength={8}
                maxLength={72}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirm-password">Confirm new password</Label>
              <Input
                id="confirm-password"
                type="password"
                autoComplete="new-password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                disabled={mutation.isPending}
                required
              />
            </div>
            {localError && (
              <div className="text-xs text-destructive">{localError}</div>
            )}
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? "Updating…" : "Update password"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Session</CardTitle>
        </CardHeader>
        <CardContent>
          <Button variant="outline" size="sm" onClick={handleLogout}>
            <LogOut className="size-4" />
            Sign out of this browser
          </Button>
          <Separator className="my-4" />
          <p className="text-xs text-muted-foreground">
            Signing out clears the tokens stored in this browser only. Other
            active sessions continue until their refresh token expires.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

function Row({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="grid grid-cols-[120px_1fr] items-center gap-3">
      <div className="text-xs uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      <div className={mono ? "font-mono text-xs" : "capitalize"}>{value}</div>
    </div>
  );
}
