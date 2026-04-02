import React from 'react';
import {TouchableOpacity, Text, View, StyleSheet} from 'react-native';
import {createNativeStackNavigator} from '@react-navigation/native-stack';
import {useTranslation} from 'react-i18next';
import {useNavigation} from '@react-navigation/native';
import type {NativeStackNavigationProp} from '@react-navigation/native-stack';

import {HomeScreen} from '../screens/HomeScreen';
import {ServerListScreen} from '../screens/ServerListScreen';
import {SettingsScreen} from '../screens/SettingsScreen';
import {AccountScreen} from '../screens/AccountScreen';
import {PaymentScreen} from '../screens/PaymentScreen';
import {colors, typography, spacing} from '../theme';

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
        options={({navigation}) => ({
          headerTitle: '',
          headerLeft: () => (
            <TouchableOpacity
              onPress={() => navigation.navigate('Settings')}
              style={navStyles.headerBtn}>
              <Text style={navStyles.headerIcon}>&#9881;</Text>
            </TouchableOpacity>
          ),
          headerRight: () => (
            <TouchableOpacity
              onPress={() => navigation.navigate('Account')}
              style={navStyles.headerBtn}>
              <Text style={navStyles.headerIcon}>&#9786;</Text>
            </TouchableOpacity>
          ),
        })}
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

const navStyles = StyleSheet.create({
  headerBtn: {
    padding: spacing.sm,
  },
  headerIcon: {
    fontSize: 22,
    color: colors.textPrimary,
  },
});
