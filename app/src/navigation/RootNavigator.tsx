import React from 'react';
import {createNativeStackNavigator} from '@react-navigation/native-stack';
import {useTranslation} from 'react-i18next';

import {HomeScreen} from '../screens/HomeScreen';
import {ServerListScreen} from '../screens/ServerListScreen';
import {SettingsScreen} from '../screens/SettingsScreen';
import {AccountScreen} from '../screens/AccountScreen';
import {PaymentScreen} from '../screens/PaymentScreen';
import {colors, typography} from '../theme';

export type RootStackParamList = {
  Home: undefined;
  ServerList: undefined;
  Settings: undefined;
  Account: undefined;
  Payment: undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();

const screenOptions = {
  headerStyle: {backgroundColor: colors.background},
  headerTintColor: colors.textPrimary,
  headerTitleStyle: typography.bodyBold,
  headerShadowVisible: false,
  contentStyle: {backgroundColor: colors.background},
};

export function RootNavigator() {
  const {t} = useTranslation();

  return (
    <Stack.Navigator screenOptions={screenOptions}>
      <Stack.Screen
        name="Home"
        component={HomeScreen}
        options={{
          headerTitle: '',
          headerLeft: () => null,
          // Settings and Account buttons will be added as headerRight
        }}
      />
      <Stack.Screen
        name="ServerList"
        component={ServerListScreen}
        options={{title: t('servers.title')}}
      />
      <Stack.Screen
        name="Settings"
        component={SettingsScreen}
        options={{title: t('settings.title')}}
      />
      <Stack.Screen
        name="Account"
        component={AccountScreen}
        options={{title: t('account.title')}}
      />
      <Stack.Screen
        name="Payment"
        component={PaymentScreen}
        options={{title: t('payment.title')}}
      />
    </Stack.Navigator>
  );
}
