import { useState, useEffect, useContext } from 'react'
import { View, ScrollView, TouchableOpacity, StyleSheet, Keyboard, KeyboardAvoidingView, Platform } from 'react-native'
import { Text, XStack } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { router } from 'expo-router'
import { Calendar, Mail, Mars, Phone, Pencil, Plus, Trash2 } from 'lucide-react-native'
import i18n from '../../src/i18n'
import { useSettings } from '../../src/hooks/useSettings'
import { useAuth } from '../../src/hooks/useAuth'
import { ThemedInput, ThemedButton, ThemedDateInput, CountryPicker, GlassCard, AppleIcon } from '@/components/atoms'
import { Toast, VerificationModal } from '@/components/molecules'
import { AlertContext } from '../../src/context/AlertContext'
import { useBlurTarget } from '@/context/BlurContext'
import { getAuthAdapter } from '@/adapters/auth'
import { getAuthApi } from '@/api/auth'
import { messageForApiError } from '@/api/errors'
import type { User } from '../../src/api'

interface ProfileFormData {
  name: string
  email: string
  phoneNumber: string
  dateOfBirth: string
  country: string
  gender: string
}

export default function ManageAccountScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { colorScheme } = useSettings()
  const { user, updateUser, ensureValidToken, deleteUserAccount } = useAuth()
  const { showConfirm } = useContext(AlertContext)!

  const [formData, setFormDataState] = useState({
    name: user?.name ?? '',
    email: user?.email ?? '',
    phoneNumber: user?.phoneNumber ?? '',
    dateOfBirth: user?.dateOfBirth ?? '',
    country: user?.country ?? '',
    gender: user?.gender ?? '',
  })
  const [hasChanges, setHasChanges] = useState(false)
  const [loading, setLoading] = useState(false)
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null)
  const [verifyingField, setVerifyingField] = useState<'email' | 'phone' | null>(null)
  const [linkingApple, setLinkingApple] = useState(false)

  const setFormData = (value: ProfileFormData | ((prev: ProfileFormData) => ProfileFormData)) => {
    setFormDataState(value)
  }

  useEffect(() => {
    setFormData({
      name: user?.name ?? '',
      email: user?.email ?? '',
      phoneNumber: user?.phoneNumber ?? '',
      dateOfBirth: user?.dateOfBirth ?? '',
      country: user?.country ?? '',
      gender: user?.gender ?? '',
    })
  }, [user])

  useEffect(() => {
    if (!user) return
    const isModified =
      formData.name !== (user.name ?? '') ||
      formData.dateOfBirth !== (user.dateOfBirth ?? '') ||
      formData.country !== (user.country ?? '') ||
      formData.gender !== (user.gender ?? '')
    setHasChanges(isModified)
  }, [formData, user])

  const handleSubmit = async () => {
    setLoading(true)
    setMessage(null)
    Keyboard.dismiss()
    try {
      await ensureValidToken()
      await updateUser({
        name: formData.name,
        dateOfBirth: formData.dateOfBirth,
        country: formData.country,
        gender: formData.gender,
      } as Partial<User>)
      setMessage({ text: i18n.t('profile_updated') || 'Profile updated', type: 'success' })
    } catch {
      setMessage({ text: i18n.t('failed_update') || 'Update failed', type: 'error' })
    } finally {
      setLoading(false)
    }
  }

  const handleContactEdit = (field: 'email' | 'phone') => {
    setVerifyingField(field)
  }

  const handleContactVerified = (updatedUser: User) => {
    setFormData(p => ({
      ...p,
      email: updatedUser.email ?? p.email,
      phoneNumber: updatedUser.phoneNumber ?? p.phoneNumber,
    }))
    setVerifyingField(null)
    setMessage({ text: i18n.t('contact_updated') || 'Contact info updated', type: 'success' })
  }

  const handleContactCancel = () => {
    setVerifyingField(null)
  }

  // Connecting Apple to an account created another way. It exists for "Hide My
  // Email": that token carries a per-app relay address, so signing in with Apple
  // can never find the account by email — this is the only way to attach it, and
  // it works because the session already proves who the account belongs to.
  const handleLinkApple = async () => {
    setLinkingApple(true)
    setMessage(null)
    try {
      const result = await getAuthAdapter().signInWithApple()
      await ensureValidToken()
      await getAuthApi().linkAppleAccount(result.identityToken)
      setMessage({ text: i18n.t('link_apple_success') || 'Apple connected.', type: 'success' })
    } catch (e) {
      const msg = String(e)
      if (msg.includes('cancelled') || msg.includes('ERR_REQUEST_CANCELED')) return
      setMessage({ text: messageForApiError(e, 'err_apple_token_invalid'), type: 'error' })
    } finally {
      setLinkingApple(false)
    }
  }

  const handleDeleteAccount = () => {
    showConfirm({
      title: i18n.t('delete_account') || 'Delete My Account',
      message: i18n.t('delete_account_warning') || 'This closes your account for good. You lose access to Rumi, your profile is anonymized, connected channels are disconnected, and your sessions, commitments, vision and badges are deleted. This cannot be undone.',
      confirmLabel: i18n.t('delete_account_confirm') || 'Delete My Account',
      destructive: true,
      type: 'superError',
      onConfirm: async () => {
        setLoading(true)
        try {
          await ensureValidToken()
          await deleteUserAccount()
          router.replace('/(auth)/signin')
        } catch {
          setMessage({ text: i18n.t('failed_update') || 'Failed to delete', type: 'error' })
        } finally {
          setLoading(false)
        }
      },
    })
  }

  const renderContactRow = (field: 'email' | 'phone') => {
    const isEmail = field === 'email'
    const Icon = isEmail ? Mail : Phone
    const value = isEmail ? formData.email : formData.phoneNumber
    const label = isEmail
      ? (i18n.t('email') || 'Email')
      : (i18n.t('phone_number') || 'Phone Number')

    return (
      <View style={styles.inputGroup}>
        <XStack alignItems="center" gap={4}>
          <Icon size={16} color="#262220" />
          <Text style={styles.sectionTitle}>{label}</Text>
        </XStack>
        {value ? (
          <XStack alignItems="center" justifyContent="space-between" style={styles.contactRow}>
            <Text style={styles.contactValue}>{value}</Text>
            <TouchableOpacity onPress={() => handleContactEdit(field)} style={styles.editButton}>
              <Pencil size={16} color={colorScheme.primary} />
            </TouchableOpacity>
          </XStack>
        ) : (
          <TouchableOpacity onPress={() => handleContactEdit(field)} style={styles.addButton}>
            <Plus size={16} color={colorScheme.primary} />
            <Text style={[styles.addButtonText, { color: colorScheme.primary }]}>
              {isEmail
                ? (i18n.t('add_email') || 'Add Email')
                : (i18n.t('add_phone') || 'Add Phone Number')}
            </Text>
          </TouchableOpacity>
        )}
      </View>
    )
  }

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
    >
      <ScrollView
        style={styles.scrollArea}
        contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 100 }]}
        keyboardShouldPersistTaps="handled"
      >
        <GlassCard variant="light" borderRadius={18} padding={16} gap={24} blurTarget={blurTargetRef}>
          {renderContactRow('email')}
          {renderContactRow('phone')}

          <View style={styles.inputGroup}>
            <Text style={styles.sectionTitle}>{i18n.t('name') || 'Name'}</Text>
            <ThemedInput
              variant="light"
              value={formData.name}
              onChangeText={(t) => setFormData(p => ({ ...p, name: t }))}
              placeholder="Name"
              color="#262220"
              fontSize={15}
              borderRadius={12}
              paddingHorizontal={0}
            />
          </View>

          <View style={styles.inputGroup}>
            <Text style={styles.sectionTitle}>{i18n.t('gender') || 'Gender'}</Text>
            <View style={styles.optionsRow}>
              <ThemedButton
                variant={formData.gender === 'male' ? 'solid' : 'outline'}
                onPress={() => setFormData(p => ({ ...p, gender: 'male' }))}
                buttonStyle={{ paddingVertical: 8, paddingHorizontal: 16 }}
              >
                {i18n.t('male') || 'Male'}
              </ThemedButton>
              <ThemedButton
                variant={formData.gender === 'female' ? 'solid' : 'outline'}
                onPress={() => setFormData(p => ({ ...p, gender: 'female' }))}
                buttonStyle={{ paddingVertical: 8, paddingHorizontal: 16 }}
              >
                {i18n.t('female') || 'Female'}
              </ThemedButton>
              <ThemedButton
                variant={formData.gender === 'other' ? 'solid' : 'outline'}
                onPress={() => setFormData(p => ({ ...p, gender: 'other' }))}
                buttonStyle={{ paddingVertical: 8, paddingHorizontal: 16 }}
              >
                {i18n.t('other') || 'Other'}
              </ThemedButton>
            </View>
          </View>

          <View style={styles.inputGroup}>
            <XStack alignItems='center' gap={4}>
              <Calendar size={16} color="#262220" />
              <Text style={styles.sectionTitle}>{i18n.t('date_of_birth') || 'Date of Birth'}
              </Text>
            </XStack>
            <ThemedDateInput
              value={formData.dateOfBirth}
              onChange={(t) => setFormData(p => ({ ...p, dateOfBirth: t }))}
              placeholder={i18n.t('select_date_of_birth') || 'Select Date of Birth'}
              borderRadius={12}
            />
          </View>

          <View style={styles.inputGroup}>
            <Text style={styles.sectionTitle}>
              {i18n.t('country') || 'Country'}
            </Text>
            <CountryPicker
              value={formData.country}
              onChange={(t) => setFormData(p => ({ ...p, country: t }))}
            />
          </View>

          {Platform.OS === 'ios' && (
            <View style={styles.inputGroup}>
              <XStack alignItems="center" gap={4}>
                <AppleIcon size={16} fill="#262220" />
                <Text style={styles.sectionTitle}>{i18n.t('link_apple') || 'Sign in with Apple'}</Text>
              </XStack>
              <Text style={styles.helperText}>
                {i18n.t('link_apple_description') || 'Connect your Apple ID so you can sign in with Apple next time.'}
              </Text>
              <ThemedButton
                variant="outline"
                fullWidth
                disabled={loading || linkingApple}
                loading={linkingApple}
                onPress={handleLinkApple}
              >
                {i18n.t('link_apple_cta') || 'Connect Apple'}
              </ThemedButton>
            </View>
          )}

          {/* The only thing here that isn't editing a field. Exporting and
              deleting data moved to Manage Data, which keeps one promise this
              row breaks: everything there leaves your account standing. */}
          <TouchableOpacity style={styles.dangerLink} onPress={handleDeleteAccount} disabled={loading}>
            <Trash2 size={20} color="#ef4444" />
            <Text style={styles.dangerLinkText}>{i18n.t('delete_account') || 'Delete My Account'}</Text>
          </TouchableOpacity>
        </GlassCard>
      </ScrollView>

      {message && <Toast message={message.text} type={message.type} onClose={() => setMessage(null)} />}

      {hasChanges && (
        <View style={[styles.saveBar, { paddingBottom: insets.bottom + 16 }]}>
          <ThemedButton variant="solid" fullWidth disabled={loading} onPress={handleSubmit}>
            {i18n.t('save_changes') || 'Save Changes'}
          </ThemedButton>
        </View>
      )}

      <VerificationModal
        visible={verifyingField !== null}
        type={verifyingField ?? 'email'}
        initialValue={verifyingField === 'email' ? (user?.email ?? '') : (user?.phoneNumber ?? '')}
        onVerified={handleContactVerified}
        onCancel={handleContactCancel}
      />
    </KeyboardAvoidingView>
  )
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 100,
  },
  inputGroup: {
    gap: 8,
    maxWidth: '100%'
  },
  sectionTitle: {
    color: '#262220',
    fontSize: 13,
    fontWeight: '600',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    opacity: 0.8,
    lineHeight: 20,
  },
  helperText: {
    color: '#262220',
    fontSize: 13,
    opacity: 0.6,
    lineHeight: 18,
  },
  contactRow: {
    backgroundColor: 'rgba(0,0,0,0.03)',
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 14,
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
    height: 48,
    padding: 12
  },
  contactValue: {
    color: '#262220',
    fontSize: 15,
    flex: 1,
  },
  editButton: {
    padding: 4,
  },
  addButton: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    backgroundColor: 'rgba(0,0,0,0.03)',
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 14,
    borderWidth: 1,
    borderColor: 'rgba(0,0,0,0.08)',
    borderStyle: 'dashed',
  },
  addButtonText: {
    fontSize: 15,
    fontWeight: '500',
  },
  optionsRow: {
    flexDirection: 'row',
    gap: 10,
    width: '100%',
  },
  dangerLink: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    paddingVertical: 12,
  },
  dangerLinkText: {
    color: '#ef4444',
    fontSize: 14,
    fontWeight: '500',
  },
  saveBar: {
    position: 'absolute',
    bottom: 0,
    left: 0,
    right: 0,
    padding: 16,
    paddingBottom: 40,
  },
})
