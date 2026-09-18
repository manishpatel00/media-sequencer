export default function Logo({ size = 32, className = '' }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      <rect x="4" y="4" width="10" height="10" rx="2" fill="var(--color-hairline)" />
      <rect x="18" y="4" width="10" height="10" rx="2" fill="var(--color-hairline)" />
      <rect x="4" y="18" width="10" height="10" rx="2" fill="var(--color-hairline)" />
      <rect x="18" y="18" width="10" height="10" rx="2" fill="var(--color-primary)" />
      
      {/* Pulse effect on the active window to indicate sync */}
      <rect 
        x="18" 
        y="18" 
        width="10" 
        height="10" 
        rx="2" 
        fill="var(--color-primary)" 
      >
        <animate 
          attributeName="opacity" 
          values="1;0.4;1" 
          dur="2s" 
          repeatCount="indefinite" 
        />
      </rect>
    </svg>
  )
}
