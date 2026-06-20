import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { QUERY_KEYS } from '@/config/constants'
import {
  clearAll,
  deleteNotification,
  listNotifications,
  markAllRead,
  markRead,
} from './api'

export function useNotifications() {
  return useQuery({
    queryKey: QUERY_KEYS.notifications,
    queryFn: listNotifications,
  })
}

export function useMarkRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => markRead(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.notifications })
    },
  })
}

export function useMarkAllRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: markAllRead,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.notifications })
    },
  })
}

export function useDeleteNotification() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deleteNotification(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.notifications })
    },
  })
}

export function useClearNotifications() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: clearAll,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: QUERY_KEYS.notifications })
    },
  })
}
