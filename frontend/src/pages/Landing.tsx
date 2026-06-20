import { Link } from "react-router"
import { useQuery } from "@tanstack/react-query"
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
    title: "极速传输",
    desc: "全球 CDN 加速，上传下载快如闪电，毫秒级响应体验。",
  },
  {
    icon: IconLink,
    title: "永久外链",
    desc: "稳定 HTTPS 直链，外链永久有效，可嵌入任意网站。",
  },
  {
    icon: IconShieldLock,
    title: "安全可靠",
    desc: "私密图片支持访问令牌保护，杜绝未授权访问。",
  },
  {
    icon: IconChartBar,
    title: "数据统计",
    desc: "实时浏览量分析，掌握每张图片的传播效果。",
  },
  {
    icon: IconFolder,
    title: "智能管理",
    desc: "公开与私密切换自如，相册式整理一目了然。",
  },
  {
    icon: IconDevices,
    title: "全平台",
    desc: "网页、开放 API、桌面客户端，多端无缝协同。",
  },
] as const

export default function LandingPage() {
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
    { value: stats ? formatNumber(stats.images) : "—", label: "托管图片" },
    { value: stats ? formatNumber(stats.users) : "—", label: "注册用户" },
    { value: stats ? formatNumber(stats.views) : "—", label: "累计浏览" },
    { value: stats ? formatBytes(stats.storage_used) : "—", label: "存储容量" },
  ]

  return (
    <div className="mx-auto max-w-5xl px-4 py-8">
      {/* Hero */}
      <section className="overflow-hidden rounded-3xl bg-gradient-to-br from-[#4A3426] to-[#8B5E3C] px-6 py-16 text-center shadow-xl sm:px-10 sm:py-20">
        <div className="mx-auto flex max-w-2xl flex-col items-center">
          <Badge className="h-auto border-white/20 bg-white/10 px-4 py-1.5 text-sm font-medium text-white backdrop-blur-sm">
            ⚡ 极速 · 稳定 · 免费
          </Badge>

          <h1 className="mt-6 font-heading text-4xl font-bold tracking-tight text-white sm:text-5xl md:text-6xl">
            你的图片
            <span className="bg-gradient-to-r from-amber-100 to-amber-300 bg-clip-text text-transparent">
              交给云端守护
            </span>
          </h1>

          <p className="mt-5 max-w-xl text-base text-white/80 sm:text-lg">
            高速稳定的图片托管服务，全球 CDN 加速，外链永久有效
          </p>

          <div className="mt-8 flex flex-col gap-3 sm:flex-row">
            <Button
              asChild
              size="lg"
              className="bg-amber-50 text-[#4A3426] hover:bg-amber-100"
            >
              <Link to={uploadHref}>
                <IconUpload className="size-4" />
                立即上传
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
                免费注册
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
        {FEATURES.map(({ icon: Icon, title, desc }) => (
          <Card
            key={title}
            className="gap-4 p-6 transition-all duration-200 hover:-translate-y-1 hover:shadow-lg"
          >
            <div className="flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Icon className="size-6" />
            </div>
            <div className="space-y-1.5">
              <h3 className="font-heading text-base font-semibold">{title}</h3>
              <p className="text-sm text-muted-foreground">{desc}</p>
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
        <p>© kserks 2026</p>
      </footer>
    </div>
  )
}
