import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// shadcn's canonical class helper — merges tailwind classes with clsx
// conditional syntax, de-duplicating conflicting utilities.
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
