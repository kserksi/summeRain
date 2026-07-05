// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

import {
  IconCircle,
  IconCircleCheck,
  IconEye,
  IconLock,
  IconWorld,
} from '@tabler/icons-react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { Image } from '@/lib/types'

export interface ImageCardProps {
  image: Image
  selectMode?: boolean
  selected?: boolean
  onToggleSelect?: (id: number) => void
}

export function ImageCard({
  image,
  selectMode = false,
  selected = false,
  onToggleSelect,
}: ImageCardProps) {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const src = `/i/${image.unique_link}.webp?w=400`
  const isPrivate = image.visibility === 'private'

  const handleClick = () => {
    if (selectMode) {
      onToggleSelect?.(image.id)
    } else {
      navigate(`/images/${image.id}`)
    }
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      className={cn(
        'group relative block w-full overflow-hidden rounded-3xl bg-card text-left ring-1 transition-all hover:-translate-y-1 hover:shadow-xl',
        selectMode && selected
          ? 'ring-2 ring-primary'
          : 'ring-border',
      )}
    >
      <div className="aspect-square w-full overflow-hidden bg-muted">
        <img
          src={src}
          alt={image.title || image.filename}
          loading="lazy"
          className="size-full object-cover transition-transform duration-300 group-hover:scale-105"
        />
      </div>

      {selectMode && (
        <div className="absolute top-2 left-2 z-10">
          {selected ? (
            <IconCircleCheck className="size-7 text-primary drop-shadow-lg" />
          ) : (
            <IconCircle className="size-7 text-white/90 drop-shadow-lg" />
          )}
        </div>
      )}

      <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent p-3 opacity-0 transition-opacity group-hover:opacity-100">
        <p className="truncate text-sm font-medium text-white">
          {image.filename}
        </p>
      </div>

      {!selectMode && (
        <div className="absolute top-2 left-2">
          <Badge
            variant={isPrivate ? 'secondary' : 'default'}
            className="backdrop-blur"
          >
            {isPrivate ? <IconLock /> : <IconWorld />}
            {isPrivate ? t('images.shared.private') : t('images.shared.public')}
          </Badge>
        </div>
      )}

      <div className="absolute top-2 right-2 flex items-center gap-1 rounded-full bg-black/50 px-2 py-0.5 text-xs font-medium text-white backdrop-blur">
        <IconEye className="size-3" />
        {image.view_count}
      </div>
    </button>
  )
}
