import { useEffect, useState } from "react";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";

import { listUsers, type AdminUser } from "@/api/users";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDate, shortId } from "@/lib/format";
import { cn } from "@/lib/utils";
import { roleBadgeClass, tierBadgeClass, tierLabel } from "@/lib/tier";

const PAGE_SIZE = 25;

// Debounce hook. Inlined here because it's the only place we need one
// and a dedicated module would be overkill for a ~6-line function.
function useDebounced<T>(value: T, ms: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return debounced;
}

export function Users() {
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [searchInput, setSearchInput] = useState("");
  const search = useDebounced(searchInput.trim(), 300);

  // Reset to page 1 whenever the (debounced) search term changes so the
  // admin doesn't land on a page beyond the filtered result count.
  useEffect(() => {
    setPage(1);
  }, [search]);

  const { data, isLoading, isFetching, isError, error } = useQuery({
    queryKey: ["admin", "users", { page, search }],
    queryFn: () => listUsers({ page, limit: PAGE_SIZE, search }),
    placeholderData: keepPreviousData,
  });

  const users = data?.users ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 0;
  const total = pagination?.total ?? 0;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">Пользователи</h1>
        <p className="text-sm text-muted-foreground">
          Найдите пользователя по ID (вставьте UUID из Telegram), чтобы
          активировать или продлить его подписку.
        </p>
      </div>

      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="Поиск по ID пользователя (вставьте из Telegram)"
            className="pl-8"
            autoFocus
          />
        </div>
        <div className="text-xs text-muted-foreground">
          {isFetching && !isLoading ? "Обновление…" : `всего: ${total}`}
        </div>
      </div>

      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[120px]">ID</TableHead>
              <TableHead>Имя</TableHead>
              <TableHead className="w-[120px]">Тариф</TableHead>
              <TableHead className="w-[100px]">Роль</TableHead>
              <TableHead className="w-[180px]">Истекает</TableHead>
              <TableHead className="w-[180px]">Создан</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              // Show a fixed number of skeleton rows to avoid layout jitter
              // on the first render.
              Array.from({ length: 6 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell colSpan={6}>
                    <Skeleton className="h-5 w-full" />
                  </TableCell>
                </TableRow>
              ))
            ) : users.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="text-center text-sm text-muted-foreground"
                >
                  {search
                    ? `Нет пользователей по запросу «${search}»`
                    : "Пока нет пользователей. Они появятся здесь после первого гостевого входа."}
                </TableCell>
              </TableRow>
            ) : (
              users.map((u) => (
                <UserRow
                  key={u.id}
                  user={u}
                  onClick={() => navigate(`/users/${u.id}`)}
                />
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {isError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
          Не удалось загрузить пользователей: {(error as Error).message}
        </div>
      )}

      {!isLoading && total > PAGE_SIZE && (
        <div className="flex items-center justify-between text-sm">
          <div className="text-muted-foreground">
            Страница {pagination?.page ?? 1} из {totalPages}
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

function UserRow({
  user,
  onClick,
}: {
  user: AdminUser;
  onClick: () => void;
}) {
  return (
    <TableRow
      onClick={onClick}
      className="cursor-pointer"
      // A11y: tables aren't naturally focusable, so the row doubles as a
      // button via keyboard handlers. Prevents the need for a dedicated
      // trailing "Open" button on every row.
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick();
        }
      }}
    >
      <TableCell className="font-mono text-xs text-muted-foreground">
        {shortId(user.id)}
      </TableCell>
      <TableCell className="font-medium">{user.full_name || "—"}</TableCell>
      {/* Tier column renders product brand names in English; role
          column capitalizes the role string as-is. */}
      <TableCell>
        <span
          className={cn(
            "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium",
            tierBadgeClass(user.subscription_tier),
          )}
        >
          {tierLabel(user.subscription_tier)}
        </span>
      </TableCell>
      <TableCell>
        <span
          className={cn(
            "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium capitalize",
            roleBadgeClass(user.role),
          )}
        >
          {user.role}
        </span>
      </TableCell>
      <TableCell className="text-muted-foreground">
        {formatDate(user.subscription_expires_at)}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {formatDate(user.created_at)}
      </TableCell>
    </TableRow>
  );
}
