import React, { createContext, useState, useContext, ReactNode } from 'react';

interface Notification {
  id: number;
  message: string;
}

interface User {
  id: number;
  name: string;
  role: 'user' | 'admin';
  swimmers: Swimmer[];
  billingInfo: BillingInfo;
  // Add other user-specific fields here
}

interface Swimmer {
  id: number;
  name: string;
  age: number;
  group: string;
  performance: Performance[];
  // Add other swimmer-specific fields here
}

interface BillingInfo {
  nextPayment: string;
  autopay: boolean;
  // Add other billing-specific fields here
}

interface Performance {
  eventId: number;
  time: string;
  date: string;
  // Add other performance-specific fields here
}

interface AppContextProps {
  user: User | null;
  notifications: Notification[];
  addNotification: (notification: Notification) => void;
  clearNotifications: () => void;
  // Add other global state functions here
}

const AppContext = createContext<AppContextProps | undefined>(undefined);

export const AppProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [notifications, setNotifications] = useState<Notification[]>([]);

  const addNotification = (notification: Notification) => {
    setNotifications((prevNotifications) => [...prevNotifications, notification]);
  };

  const clearNotifications = () => {
    setNotifications([]);
  };

  return (
    <AppContext.Provider value={{ user, notifications, addNotification, clearNotifications }}>
      {children}
    </AppContext.Provider>
  );
};

export const useAppContext = (): AppContextProps => {
  const context = useContext(AppContext);
  if (!context) {
    throw new Error('useAppContext must be used within an AppProvider');
  }
  return context;
};
