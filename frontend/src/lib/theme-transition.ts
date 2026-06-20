const BG_MAP: Record<string, string> = {
  light: '#F1E7DA',
  dark: '#16100D',
}

export function transitionTheme(
  theme: 'light' | 'dark',
  originX: number,
  originY: number,
  apply: () => void,
): void {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    apply()
    return
  }
  if (document.startViewTransition) {
    document.documentElement.style.setProperty('--cx', `${originX}px`)
    document.documentElement.style.setProperty('--cy', `${originY}px`)
    document.startViewTransition(apply)
    return
  }
  // Fallback: WAAPI mask scale
  const mask = document.createElement('div')
  mask.style.cssText = `position:fixed;width:300vmax;height:300vmax;border-radius:50%;left:${originX}px;top:${originY}px;margin-left:-150vmax;margin-top:-150vmax;z-index:9999;pointer-events:none;background:${BG_MAP[theme] ?? '#000'}`
  document.body.appendChild(mask)
  const anim = mask.animate(
    [{ transform: 'scale(0)' }, { transform: 'scale(1)' }],
    { duration: 550, easing: 'cubic-bezier(.2,.7,.2,1)' },
  )
  anim.onfinish = () => { apply(); mask.remove() }
}
