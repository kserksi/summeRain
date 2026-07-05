// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react'
import { Link } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import {
  IconAlertCircle,
  IconEye,
  IconEyeOff,
  IconLoader2,
  IconUserPlus,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import i18n from '@/i18n'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ROUTES, RECAPTCHA_ACTIONS } from '@/config/constants'
import { useErrorMessage, useRegister } from '@/features/auth/hooks'
import { Captcha } from '@/features/captcha/components/Captcha'
import type { CaptchaPayload } from '@/lib/types'

const registerSchema = z
  .object({
    username: z
      .string()
      .min(3, i18n.t('auth.validation.usernameMin3'))
      .max(50, i18n.t('auth.validation.usernameMax50')),
    email: z.string().email(i18n.t('auth.validation.emailInvalid')),
    password: z.string().min(8, i18n.t('auth.validation.passwordMin8')),
    confirmPassword: z.string().min(1, i18n.t('auth.validation.confirmRequired')),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: i18n.t('auth.validation.passwordMismatch'),
    path: ['confirmPassword'],
  })

type RegisterFormData = z.infer<typeof registerSchema>

export default function RegisterPage() {
  const { t } = useTranslation()
  const registerMutation = useRegister()
  const resolveError = useErrorMessage()
  const [showPassword, setShowPassword] = useState(false)
  const [captchaPayload, setCaptchaPayload] = useState<CaptchaPayload | undefined>(undefined)

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<RegisterFormData>({
    resolver: zodResolver(registerSchema),
    defaultValues: { username: '', email: '', password: '', confirmPassword: '' },
  })

  const onSubmit = (data: RegisterFormData) => {
    registerMutation.mutate({
      username: data.username,
      email: data.email,
      password: data.password,
      captcha: captchaPayload,
    })
  }

  return (
    <div className="grid min-h-[calc(100vh-8rem)] place-items-center rounded-3xl bg-gradient-to-br from-[#6F4E37] via-[#5C3D26] to-[#33261B] p-4">
      <Card className="w-full max-w-md rounded-3xl shadow-2xl">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-semibold">{t('common.register')}</CardTitle>
          <CardDescription>{t('auth.registerSubtitle', { appName: t('common.appName') })}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4" noValidate>
            {registerMutation.isError && (
              <Alert variant="destructive">
                <IconAlertCircle />
                <AlertDescription>{resolveError(registerMutation.error)}</AlertDescription>
              </Alert>
            )}

            <div className="flex flex-col gap-2">
              <Label htmlFor="username">{t('auth.field.username')}</Label>
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
              <Label htmlFor="email">{t('auth.field.email')}</Label>
              <Input
                id="email"
                type="email"
                autoComplete="email"
                aria-invalid={!!errors.email}
                {...register('email')}
              />
              {errors.email && (
                <p className="text-xs text-destructive">{errors.email.message}</p>
              )}
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="password">{t('auth.field.password')}</Label>
              <div className="relative">
                <Input
                  id="password"
                  className="pr-9"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="new-password"
                  aria-invalid={!!errors.password}
                  {...register('password')}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="absolute top-1/2 right-1 -translate-y-1/2"
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? t('auth.hidePassword') : t('auth.showPassword')}
                  tabIndex={-1}
                >
                  {showPassword ? <IconEyeOff /> : <IconEye />}
                </Button>
              </div>
              {errors.password && (
                <p className="text-xs text-destructive">{errors.password.message}</p>
              )}
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="confirmPassword">{t('auth.field.confirmPassword')}</Label>
              <Input
                id="confirmPassword"
                type={showPassword ? 'text' : 'password'}
                autoComplete="new-password"
                aria-invalid={!!errors.confirmPassword}
                {...register('confirmPassword')}
              />
              {errors.confirmPassword && (
                <p className="text-xs text-destructive">{errors.confirmPassword.message}</p>
              )}
            </div>

            <Captcha action={RECAPTCHA_ACTIONS.REGISTER} onVerified={setCaptchaPayload} />

            <Button type="submit" className="w-full" disabled={registerMutation.isPending}>
              {registerMutation.isPending ? (
                <IconLoader2 className="animate-spin" />
              ) : (
                <IconUserPlus />
              )}
              {t('common.register')}
            </Button>
          </form>

          <p className="mt-4 text-center text-sm text-muted-foreground">
            {t('auth.hasAccount')}{' '}
            <Link to={ROUTES.LOGIN} className="font-medium text-primary hover:underline">
              {t('common.login')}
            </Link>
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
