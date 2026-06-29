// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { cn } from "@/lib/utils"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn("animate-pulse rounded-xl bg-muted", className)}
      {...props}
    />
  )
}

export { Skeleton }
