import { useState, useMemo, useRef, useCallback, useEffect } from 'react'
import { View, type ViewStyle } from 'react-native'
import { createPortal } from 'react-dom'
import { Search, Check as CheckIcon } from 'lucide-react-native'
import i18n from '@/i18n'
import { ThemedInput } from '@/components/atoms/ThemedInput'
import { COUNTRIES } from '@/utils/countries'
import { INK } from '@/styles/glass'

interface CountryPickerProps {
  value: string
  onChange: (country: string) => void
  placeholder?: string
}

export function CountryPicker({ value, onChange, placeholder }: CountryPickerProps) {
  const [showDropdown, setShowDropdown] = useState(false)
  const [search, setSearch] = useState('')
  const inputRef = useRef<View>(null)
  const blurTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [dropdownPos, setDropdownPos] = useState<{ top: number; left: number; width: number } | null>(null)

  const selectedCountry = useMemo(
    () => COUNTRIES.find(c => c.name === value || c.code === value),
    [value]
  )

  const filteredCountries = useMemo(() => {
    if (!search.trim()) return COUNTRIES
    const q = search.toLowerCase()
    return COUNTRIES.filter(c =>
      c.name.toLowerCase().includes(q) || c.code.toLowerCase().includes(q)
    )
  }, [search])

  const updateDropdownPosition = useCallback(() => {
    const el = inputRef.current as any
    if (!el) return
    const rect = el.getBoundingClientRect()
    setDropdownPos({ top: rect.bottom + 4, left: rect.left, width: rect.width })
  }, [])

  const openDropdown = useCallback(() => {
    setShowDropdown(true)
    setSearch('')
    requestAnimationFrame(() => updateDropdownPosition())
  }, [updateDropdownPosition])

  const closeDropdown = useCallback(() => {
    if (blurTimeoutRef.current) { clearTimeout(blurTimeoutRef.current); blurTimeoutRef.current = null }
    setShowDropdown(false)
    setSearch('')
    setDropdownPos(null)
  }, [])

  const handleSelect = (countryName: string) => {
    if (blurTimeoutRef.current) { clearTimeout(blurTimeoutRef.current); blurTimeoutRef.current = null }
    onChange(countryName)
    closeDropdown()
  }

  useEffect(() => {
    if (!showDropdown) return
    const handleMouseDown = (e: MouseEvent) => {
      const target = e.target as Node
      if (inputRef.current && (inputRef.current as any).contains(target)) return
      const portal = document.querySelector('[data-country-dropdown]')
      if (portal && portal.contains(target)) return
      closeDropdown()
    }
    const handleScroll = () => { updateDropdownPosition() }
    const handleResize = () => { updateDropdownPosition() }
    document.addEventListener('mousedown', handleMouseDown)
    window.addEventListener('scroll', handleScroll, true)
    window.addEventListener('resize', handleResize)
    return () => {
      document.removeEventListener('mousedown', handleMouseDown)
      window.removeEventListener('scroll', handleScroll, true)
      window.removeEventListener('resize', handleResize)
    }
  }, [showDropdown, closeDropdown, updateDropdownPosition])

  return (
    <View ref={inputRef}>
      <ThemedInput
        value={showDropdown ? search : (selectedCountry?.name ?? '')}
        onFocus={() => {
          if (blurTimeoutRef.current) { clearTimeout(blurTimeoutRef.current); blurTimeoutRef.current = null }
          openDropdown()
        }}
        onChangeText={(t) => {
          setSearch(t)
          setShowDropdown(true)
          requestAnimationFrame(() => updateDropdownPosition())
        }}
        onBlur={() => {
          blurTimeoutRef.current = setTimeout(() => {
            if (showDropdown && !search) {
              closeDropdown()
            }
          }, 150)
        }}
        placeholder={placeholder || (i18n.t('select_country') || 'Select Country')}
        aria-label={i18n.t('search_countries') || 'Search countries...'}
        placeholderTextColor={INK.primary}
        color={INK.primary}
        fontSize={15}
        icon={<Search size={18} color={INK.primary} />}
        style={{ height: 44 } as ViewStyle}
        inputSyle={{ paddingLeft: 0 }}
        variant='light'
      />
      {showDropdown && dropdownPos && typeof document !== 'undefined' && createPortal(
        <div
          data-country-dropdown
          style={{
            position: 'fixed',
            top: dropdownPos.top,
            left: dropdownPos.left,
            width: dropdownPos.width,
            maxHeight: 200,
            overflowY: 'auto',
            borderRadius: 12,
            border: '1px solid rgba(0,0,0,0.1)',
            backgroundColor: 'rgba(255,255,255,0.95)',
            backdropFilter: 'blur(8px)',
            WebkitBackdropFilter: 'blur(8px)',
            boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
            zIndex: 9999,
          }}
        >
          {filteredCountries.map((item) => {
            const isSelected = item.code === (selectedCountry?.code || '')
            return (
              <div
                key={item.code}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  padding: '4px 8px',
                  gap: 8,
                  cursor: 'pointer',
                  backgroundColor: isSelected ? 'rgba(16,185,129,0.1)' : 'transparent',
                  borderRadius: 8,
                  margin: '2px 4px',
                }}
                onMouseEnter={(e) => {
                  if (!isSelected) (e.currentTarget as HTMLDivElement).style.backgroundColor = 'rgba(0,0,0,0.04)'
                }}
                onMouseLeave={(e) => {
                  if (!isSelected) (e.currentTarget as HTMLDivElement).style.backgroundColor = 'transparent'
                }}
                onMouseDown={(e) => {
                  e.preventDefault()
                  handleSelect(item.name)
                }}
              >
                <span style={{ fontSize: 22 }}>{item.flag}</span>
                <span style={{ flex: 1, color: '#1a1a1a', fontSize: 14, fontWeight: isSelected ? '600' : '400' }}>
                  {item.name}
                </span>
                {isSelected && <CheckIcon size={18} color="#10b981" />}
              </div>
            )
          })}
        </div>,
        document.body
      )}
    </View>
  )
}
