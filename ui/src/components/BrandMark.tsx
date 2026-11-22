/**
 * The Samplier mark: a single sample droplet with a meniscus fill line — the
 * simplest form that still reads as "measured liquid". Rendered as inline SVG
 * so it stays crisp and needs no network request.
 */
export function BrandMark({
  size = 32,
  tiled = true,
  title = 'Samplier',
}: {
  size?: number
  /** Draw the rounded dark tile (favicon / dark chrome) or the bare droplet. */
  tiled?: boolean
  title?: string
}) {
  const drop = (
    <>
      <path
        d="M32 6.5c0 0 18.5 23.8 18.5 33.5a18.5 18.5 0 0 1-37 0C13.5 30.3 32 6.5 32 6.5Z"
        fill="url(#sm-drop)"
      />
      <path
        d="M16.2 35.4c5.2 4.6 10.6 6.9 15.8 6.9s10.6-2.3 15.8-6.9"
        fill="none"
        stroke="#0a0f13"
        strokeWidth="3"
        strokeLinecap="round"
      />
    </>
  )

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      role="img"
      aria-label={title}
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <linearGradient id="sm-tile" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#101d1a" />
          <stop offset="0.55" stopColor="#0b1216" />
          <stop offset="1" stopColor="#0a1420" />
        </linearGradient>
        <linearGradient id="sm-drop" x1="0" y1="0" x2="0.65" y2="1">
          <stop offset="0" stopColor="#5ee7a8" />
          <stop offset="0.55" stopColor="#35b37e" />
          <stop offset="1" stopColor="#2f7fb0" />
        </linearGradient>
      </defs>

      {tiled && (
        <>
          <rect width="64" height="64" rx="15" fill="url(#sm-tile)" />
          <rect
            x="0.75"
            y="0.75"
            width="62.5"
            height="62.5"
            rx="14.25"
            fill="none"
            stroke="#1d2f36"
            strokeWidth="1.5"
          />
          <g transform="translate(32 32.6) scale(0.72) translate(-32 -32)">{drop}</g>
        </>
      )}
      {!tiled && drop}
    </svg>
  )
}
