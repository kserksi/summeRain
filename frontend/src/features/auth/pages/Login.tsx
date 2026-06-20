import { useState } from 'react'
import { Link } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { IconAlertCircle, IconEye, IconEyeOff, IconLoader2, IconLogin } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ROUTES, RECAPTCHA_ACTIONS } from '@/config/constants'
import { useErrorMessage, useLogin } from '@/features/auth/hooks'
import { Captcha } from '@/features/captcha/components/Captcha'
import type { CaptchaPayload } from '@/lib/types'

const loginSchema = z.object({
  username: z.string().min(1, '请输入用户名'),
  password: z.string().min(6, '密码至少 6 位'),
})

type LoginFormData = z.infer<typeof loginSchema>

export default function LoginPage() {
  const { t } = useTranslation()
  const login = useLogin()
  const resolveError = useErrorMessage()
  const [showPassword, setShowPassword] = useState(false)
  const [captchaPayload, setCaptchaPayload] = useState<CaptchaPayload | undefined>(undefined)

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: '', password: '' },
  })

  const onSubmit = (data: LoginFormData) => {
    login.mutate({ ...data, captcha: captchaPayload })
  }

  return (
    <div className="grid min-h-[calc(100vh-8rem)] place-items-center rounded-3xl bg-gradient-to-br from-[#6F4E37] via-[#5C3D26] to-[#33261B] p-4">
      <Card className="w-full max-w-md rounded-3xl shadow-2xl">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-semibold">{t('common.login')}</CardTitle>
          <CardDescription>登录你的 ImgCloud 账号</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4" noValidate>
            {login.isError && (
              <Alert variant="destructive">
                <IconAlertCircle />
                <AlertDescription>{resolveError(login.error)}</AlertDescription>
              </Alert>
            )}

            <div className="flex flex-col gap-2">
              <Label htmlFor="username">用户名</Label>
              <Input
                id="username"
                autoComplete="username"
                aria-invalid={!!errors.username}
                {...register('username')}
              />
              {errors.username && (
                <p className="text-xs text-destructive">{errors.username.message}</p>
              )}
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="password">密码</Label>
              <div className="relative">
                <Input
                  id="password"
                  className="pr-9"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="current-password"
                  aria-invalid={!!errors.password}
                  {...register('password')}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="absolute top-1/2 right-1 -translate-y-1/2"
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? '隐藏密码' : '显示密码'}
                  tabIndex={-1}
                >
                  {showPassword ? <IconEyeOff /> : <IconEye />}
                </Button>
              </div>
              {errors.password && (
                <p className="text-xs text-destructive">{errors.password.message}</p>
              )}
            </div>

            <Captcha action={RECAPTCHA_ACTIONS.LOGIN} onVerified={setCaptchaPayload} />

            <Button type="submit" className="w-full" disabled={login.isPending}>
              {login.isPending ? (
                <IconLoader2 className="animate-spin" />
              ) : (
                <IconLogin />
              )}
              {t('common.login')}
            </Button>
          </form>

          <p className="mt-4 text-center text-sm text-muted-foreground">
            还没有账号？{' '}
            <Link
              to={ROUTES.REGISTER}
              className="font-medium text-primary hover:underline"
            >
              立即注册
            </Link>
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
