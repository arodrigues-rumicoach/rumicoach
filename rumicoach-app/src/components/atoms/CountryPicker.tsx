import { useState, useMemo, useCallback, memo } from 'react'
import { Modal, FlatList, TouchableOpacity, Keyboard, Pressable, KeyboardAvoidingView, View } from 'react-native'
import { YStack, XStack, Text } from 'tamagui'
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

export const CountryPicker = memo(function CountryPicker({ value, onChange, placeholder }: CountryPickerProps) {
  const [showPicker, setShowPicker] = useState(false)
  const [search, setSearch] = useState('')

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

  const handleSelect = useCallback((countryName: string) => {
    onChange(countryName)
    setShowPicker(false)
    setSearch('')
  }, [onChange])

  const handleClose = useCallback(() => {
    setShowPicker(false)
    setSearch('')
  }, [])

  return (
    <View style={{ width: '100%' }}>
      <TouchableOpacity onPress={() => setShowPicker(true)} activeOpacity={0.7} style={{ width: '100%' }}>
        <XStack
          height={44}
          backgroundColor="rgba(255,255,255,0.05)"
          borderWidth={1}
          borderColor={INK.primary}
          borderRadius={12}
          paddingHorizontal={14}
          alignItems="center"
          gap="$2"
        >
          {selectedCountry ? (
            <>
              <Text fontSize={20}>{selectedCountry.flag}</Text>
              <Text color="$onGlass" fontSize={14}>{selectedCountry.name}</Text>
            </>
          ) : (
            <Text color="$onGlass" fontSize={14}>
              {placeholder || (i18n.t('select_country') || 'Select Country')}
            </Text>
          )}
        </XStack>
      </TouchableOpacity>

      <Modal visible={showPicker} transparent animationType="slide" onRequestClose={handleClose}>
        <KeyboardAvoidingView style={{ flex: 1 }} behavior="padding">
          <Pressable style={{ flex: 1 }} onPress={() => { Keyboard.dismiss(); handleClose() }}>
            <YStack flex={1} justifyContent="flex-end">
              <YStack backgroundColor='#fff' borderTopLeftRadius={20} borderTopRightRadius={20} maxHeight="80%" paddingBottom="$8">
                <XStack padding="$4" borderBottomWidth={1} borderBottomColor="rgba(255,255,255,0.08)" alignItems="center" justifyContent="space-between">
                  <Text color={INK.primary} fontSize={18} fontWeight="bold">
                    {(i18n.t('select_country') || 'Select Country')}
                  </Text>
                  <TouchableOpacity onPress={handleClose}>
                    <Text color={INK.primary} fontSize={14}>{(i18n.t('close') || 'Close')}</Text>
                  </TouchableOpacity>
                </XStack>

                <ThemedInput
                  placeholder={i18n.t('search_countries') || 'Search countries...'}
                  value={search}
                  onChangeText={setSearch}
                  autoCapitalize="none"
                  autoCorrect={false}
                  icon={<Search size={18} />}
                  variant='light'
                />

                <FlatList
                  data={filteredCountries}
                  keyExtractor={item => item.code}
                  style={{ maxHeight: 400 }}
                  keyboardShouldPersistTaps="handled"
                  removeClippedSubviews
                  maxToRenderPerBatch={10}
                  windowSize={5}
                  renderItem={({ item }) => {
                    const isSelected = item.code === (selectedCountry?.code || '')
                    return (
                      <TouchableOpacity
                        onPress={() => handleSelect(item.name)}
                        style={{
                          flexDirection: 'row',
                          alignItems: 'center',
                          paddingHorizontal: 16,
                          paddingVertical: 12,
                          backgroundColor: isSelected ? 'rgba(16,185,129,0.1)' : 'transparent',
                        }}
                      >
                        <Text fontSize={22} marginRight="$3">{item.flag}</Text>
                        <YStack flex={1}>
                          <Text color={INK.primary} fontSize={14} fontWeight={isSelected ? '600' : '400'}>
                            {item.name}
                          </Text>
                        </YStack>
                        {isSelected && <CheckIcon size={18} color="#10b981" />}
                      </TouchableOpacity>
                    )
                  }}
                />
              </YStack>
            </YStack>
          </Pressable>
        </KeyboardAvoidingView>
      </Modal>
    </View>
  )
})
