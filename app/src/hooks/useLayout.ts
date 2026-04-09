import {useWindowDimensions} from 'react-native';

const TABLET_BREAKPOINT = 600;
const MAX_CONTENT_WIDTH = 500;

export function useLayout() {
  const {width, height} = useWindowDimensions();
  const isTablet = Math.min(width, height) >= TABLET_BREAKPOINT;
  const scale = isTablet ? 1.3 : 1;
  const contentMaxWidth = isTablet ? MAX_CONTENT_WIDTH : undefined;

  return {width, height, isTablet, scale, contentMaxWidth};
}
