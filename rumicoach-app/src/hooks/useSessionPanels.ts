import { useState, useCallback } from 'react'
import type { ActionPlanData, SessionSummaryData, SessionTaskItem } from '@/context/SessionContext'
import { User } from '@/api'

export interface SessionPanelsState {
  wheelData: never[]
  showWheel: boolean
  eisenhowerData: Record<string, unknown> | null
  showEisenhower: boolean
  actionPlanData: ActionPlanData | null
  showActionPlan: boolean
  sessionSummaryData: SessionSummaryData | null
  showSessionSummary: boolean
  sessionTasksData: SessionTaskItem[] | null
  showSessionTasks: boolean
  sessionValuesData: string[] | null
  showSessionValues: boolean
  tasksVersion: number
  showOnboardingData: boolean
}

export function useSessionPanels() {
  const [wheelData, setWheelData] = useState<never[]>([])
  const [showWheel, setShowWheel] = useState(false)
  const [eisenhowerData, setEisenhowerData] = useState<Record<string, unknown> | null>(null)
  const [showEisenhower, setShowEisenhower] = useState(false)
  const [actionPlanData, setActionPlanData] = useState<ActionPlanData | null>(null)
  const [showActionPlan, setShowActionPlan] = useState(false)
  const [sessionSummaryData, setSessionSummaryData] = useState<SessionSummaryData | null>(null)
  const [showSessionSummary, setShowSessionSummary] = useState(false)
  const [sessionTasksData, setSessionTasksData] = useState<SessionTaskItem[] | null>(null)
  const [showSessionTasks, setShowSessionTasks] = useState(false)
  const [sessionValuesData, setSessionValuesData] = useState<string[] | null>(null)
  const [showSessionValues, setShowSessionValues] = useState(false)
  const [tasksVersion, setTasksVersion] = useState(0)
  const [userWSData, setUserWSData] = useState<Partial<User> | null>(null)
  const [showOnboardingData, setShowOnboardingData] = useState(false)

  const hideAllPanels = useCallback(() => {
    setShowWheel(false)
    setShowEisenhower(false)
    setShowActionPlan(false)
    setShowSessionSummary(false)
    setShowSessionTasks(false)
    setShowSessionValues(false)
    setShowOnboardingData(false)
  }, [])

  const handleWheelUpdate = useCallback((data: never[]) => {
    setWheelData(data)
    setShowWheel(true)
    setShowEisenhower(false)
    setShowActionPlan(false)
    setShowSessionSummary(false)
    setShowSessionTasks(false)
    setShowSessionValues(false)
  }, [])

  const handleEisenhowerUpdate = useCallback((data: Record<string, unknown>) => {
    setEisenhowerData(data)
    setShowEisenhower(true)
    setShowWheel(false)
    setShowActionPlan(false)
    setShowSessionSummary(false)
    setShowSessionTasks(false)
    setShowSessionValues(false)
  }, [])

  const handleActionPlanUpdate = useCallback((data: ActionPlanData) => {
    setActionPlanData(data)
    setShowActionPlan(true)
    setShowWheel(false)
    setShowEisenhower(false)
    setShowSessionSummary(false)
    setShowSessionTasks(false)
    setShowSessionValues(false)
  }, [])

  const handleSessionSummaryUpdate = useCallback((data: SessionSummaryData) => {
    setSessionSummaryData(data)
    setShowSessionSummary(true)
    setShowWheel(false)
    setShowEisenhower(false)
    setShowActionPlan(false)
    setShowSessionTasks(false)
    setShowSessionValues(false)
  }, [])

  const handleSessionTasksUpdate = useCallback((data: SessionTaskItem[]) => {
    setSessionTasksData(data)
    setShowSessionTasks(true)
    setShowWheel(false)
    setShowEisenhower(false)
    setShowActionPlan(false)
    setShowSessionSummary(false)
    setShowSessionValues(false)
  }, [])

  // The Values session's live reveal: the user watches their chosen values land on
  // screen the moment save_top_values fires (Filipa's guiao expects the values to be
  // displayed, not just spoken back).
  const handleSessionValuesUpdate = useCallback((values: string[]) => {
    setSessionValuesData(values)
    setShowSessionValues(true)
    setShowWheel(false)
    setShowEisenhower(false)
    setShowActionPlan(false)
    setShowSessionSummary(false)
    setShowSessionTasks(false)
  }, [])

  const handleTasksUpdated = useCallback(() => {
    setTasksVersion(prev => prev + 1)
  }, [])

  const handleDisconnect = useCallback(() => {
    setShowWheel(false)
    setWheelData([])
    setShowEisenhower(false)
    setEisenhowerData(null)
    setShowActionPlan(false)
    setActionPlanData(null)
    setShowSessionTasks(false)
    setSessionTasksData(null)
    setShowSessionValues(false)
    setSessionValuesData(null)
    setShowOnboardingData(false)
    setUserWSData(null)
  }, [])

  const handleNewSession = useCallback(() => {
    setShowSessionSummary(false)
    setSessionSummaryData(null)
  }, [])

  const handleOnboardingUpdate = useCallback((data: {
    dateOfBirth?: string | null
    gender?: 'male' | 'female' | 'other' | null
    country?: string | null
  }) => {
    setUserWSData(data)
    setShowOnboardingData(true)
  }, [])

  return {
    wheelData, showWheel,
    eisenhowerData, showEisenhower,
    actionPlanData, showActionPlan,
    sessionSummaryData, showSessionSummary,
    sessionTasksData, showSessionTasks,
    sessionValuesData, showSessionValues,
    tasksVersion, userWSData, showOnboardingData,
    hideAllPanels,
    handleWheelUpdate,
    handleEisenhowerUpdate,
    handleActionPlanUpdate,
    handleSessionSummaryUpdate,
    handleSessionTasksUpdate,
    handleSessionValuesUpdate,
    handleTasksUpdated,
    handleDisconnect,
    handleNewSession,
    handleOnboardingUpdate
  }
}
