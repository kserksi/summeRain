// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useState } from 'react'
import Cropper, { type Area } from 'react-easy-crop'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Slider } from '@/components/ui/slider'

async function getCroppedImg(src: string, area: Area, size = 256): Promise<string> {
  const img = new Image()
  img.src = src
  await new Promise((r) => { img.onload = r })
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')!
  ctx.drawImage(img, area.x, area.y, area.width, area.height, 0, 0, size, size)
  return canvas.toDataURL('image/webp', 0.85)
}

export function AvatarCropDialog({
  open,
  imageSrc,
  onClose,
  onConfirm,
}: {
  open: boolean
  imageSrc: string
  onClose: () => void
  onConfirm: (dataUrl: string) => void
}) {
  const { t } = useTranslation()
  const [crop, setCrop] = useState({ x: 0, y: 0 })
  const [zoom, setZoom] = useState(1)
  const [area, setArea] = useState<Area | null>(null)
  const [busy, setBusy] = useState(false)

  const onCropComplete = useCallback((_: Area, areaPixels: Area) => {
    setArea(areaPixels)
  }, [])

  const handleConfirm = async () => {
    if (!area) return
    setBusy(true)
    try {
      const dataUrl = await getCroppedImg(imageSrc, area)
      onConfirm(dataUrl)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t('profile.avatar.cropTitle')}</DialogTitle>
        </DialogHeader>
        <div className="relative aspect-square overflow-hidden rounded-xl bg-black">
          <Cropper
            image={imageSrc}
            crop={crop}
            zoom={zoom}
            aspect={1}
            onCropChange={setCrop}
            onZoomChange={setZoom}
            onCropComplete={onCropComplete}
          />
        </div>
        <div className="flex items-center gap-3 px-1">
          <span className="text-xs text-muted-foreground">{t('profile.avatar.zoom')}</span>
          <Slider value={[zoom]} min={1} max={3} step={0.1} onValueChange={([v]) => setZoom(v)} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button onClick={handleConfirm} disabled={busy || !area}>
            {busy ? t('common.loading') : t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
