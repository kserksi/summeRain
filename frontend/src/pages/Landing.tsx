// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

import { Link } from "react-router"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  IconArrowRight,
  IconBolt,
  IconBrandCloudflare,
  IconBrandGithub,
  IconChartBar,
  IconDevices,
  IconFolder,
  IconLink,
  IconShieldLock,
  IconUpload,
  IconUserPlus,
} from "@tabler/icons-react"

import { useAuthStore } from "@/store/auth-store"
import { api } from "@/lib/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

const FEATURES = [
  {
    icon: IconBolt,
    titleKey: "landing.features.fastTransfer.title",
    descKey: "landing.features.fastTransfer.desc",
  },
  {
    icon: IconLink,
    titleKey: "landing.features.permanentLink.title",
    descKey: "landing.features.permanentLink.desc",
  },
  {
    icon: IconShieldLock,
    titleKey: "landing.features.secure.title",
    descKey: "landing.features.secure.desc",
  },
  {
    icon: IconChartBar,
    titleKey: "landing.features.analytics.title",
    descKey: "landing.features.analytics.desc",
  },
  {
    icon: IconFolder,
    titleKey: "landing.features.management.title",
    descKey: "landing.features.management.desc",
  },
  {
    icon: IconDevices,
    titleKey: "landing.features.crossPlatform.title",
    descKey: "landing.features.crossPlatform.desc",
  },
] as const

export default function LandingPage() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const uploadHref = user ? "/upload" : "/login"

  const { data: stats } = useQuery({
    queryKey: ["landing-stats"],
    queryFn: () =>
      api.get<{ images: number; users: number; views: number; storage_used: number }>(
        "/public/stats",
        { skipAuthRedirect: true },
      ),
    retry: false,
    staleTime: 60_000,
  })

  const STATS = [
    { value: stats ? formatNumber(stats.images) : "—", label: t("landing.stats.images") },
    { value: stats ? formatNumber(stats.users) : "—", label: t("landing.stats.users") },
    { value: stats ? formatNumber(stats.views) : "—", label: t("landing.stats.views") },
    { value: stats ? formatBytes(stats.storage_used) : "—", label: t("landing.stats.storage") },
  ]

  return (
    <div className="mx-auto max-w-5xl px-4 py-8">
      {/* Hero */}
      <section className="overflow-hidden rounded-3xl bg-gradient-to-br from-[#4A3426] to-[#8B5E3C] px-6 py-16 text-center shadow-xl sm:px-10 sm:py-20">
        <div className="mx-auto flex max-w-2xl flex-col items-center">
          <Badge className="h-auto border-white/20 bg-white/10 px-4 py-1.5 text-sm font-medium text-white backdrop-blur-sm">
            {t("landing.hero.badge")}
          </Badge>

          <h1 className="mt-6 font-heading text-4xl font-bold tracking-tight text-white sm:text-5xl md:text-6xl">
            {t("landing.hero.titlePrefix")}
            <span className="bg-gradient-to-r from-amber-100 to-amber-300 bg-clip-text text-transparent">
              {t("landing.hero.titleHighlight")}
            </span>
          </h1>

          <p className="mt-5 max-w-xl text-base text-white/80 sm:text-lg">
            {t("landing.hero.subtitle")}
          </p>

          <div className="mt-8 flex flex-col gap-3 sm:flex-row">
            <Button
              asChild
              size="lg"
              className="bg-amber-50 text-[#4A3426] hover:bg-amber-100"
            >
              <Link to={uploadHref}>
                <IconUpload className="size-4" />
                {t("landing.hero.uploadNow")}
                <IconArrowRight className="size-4" />
              </Link>
            </Button>
            <Button
              asChild
              size="lg"
              variant="outline"
              className="border-white/30 bg-transparent text-white hover:bg-white/10 hover:text-white"
            >
              <Link to="/register">
                <IconUserPlus className="size-4" />
                {t("landing.hero.registerFree")}
              </Link>
            </Button>
          </div>

          <div className="mt-12 grid w-full grid-cols-2 gap-4 sm:grid-cols-4">
            {STATS.map((s) => (
              <div key={s.label} className="flex flex-col items-center">
                <span className="font-heading text-2xl font-bold text-white sm:text-3xl">
                  {s.value}
                </span>
                <span className="mt-1 text-xs text-white/70 sm:text-sm">
                  {s.label}
                </span>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Features Bento */}
      <section className="mt-10 grid grid-cols-1 gap-5 md:grid-cols-3">
        {FEATURES.map(({ icon: Icon, titleKey, descKey }) => (
          <Card
            key={titleKey}
            className="gap-4 p-6 transition-all duration-200 hover:-translate-y-1 hover:shadow-lg"
          >
            <div className="flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Icon className="size-6" />
            </div>
            <div className="space-y-1.5">
              <h3 className="font-heading text-base font-semibold">{t(titleKey)}</h3>
              <p className="text-sm text-muted-foreground">{t(descKey)}</p>
            </div>
          </Card>
        ))}
      </section>

      {/* Footer */}
      <footer className="mt-10 flex flex-col items-center gap-3 text-center text-sm text-muted-foreground">
        <div className="flex items-center gap-4">
          <a
            href="https://www.cloudflare.com"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1.5 text-muted-foreground transition hover:text-foreground"
          >
            <IconBrandCloudflare className="size-4" />
            Cloudflare
          </a>
          <a
            href="https://github.com/kserksi/summerain"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1.5 text-muted-foreground transition hover:text-foreground"
          >
            <IconBrandGithub className="size-4" />
            GitHub
          </a>
        </div>
        <p>© 2026 The summeRain Authors</p>
      </footer>
    </div>
  )
}
