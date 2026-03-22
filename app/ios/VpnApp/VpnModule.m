#import <React/RCTBridgeModule.h>
#import <React/RCTEventEmitter.h>

/// Objective-C bridge that exposes the VPN module to React Native.
/// The actual implementation is in VpnModuleImpl.swift.
@interface RCT_EXTERN_MODULE(VpnModule, RCTEventEmitter)

RCT_EXTERN_METHOD(connect:(NSString *)configJSON
                  resolve:(RCTPromiseResolveBlock)resolve
                  reject:(RCTPromiseRejectBlock)reject)

RCT_EXTERN_METHOD(disconnect:(RCTPromiseResolveBlock)resolve
                  reject:(RCTPromiseRejectBlock)reject)

RCT_EXTERN_METHOD(getStatus:(RCTPromiseResolveBlock)resolve
                  reject:(RCTPromiseRejectBlock)reject)

RCT_EXTERN_METHOD(probeServers:(NSString *)serversJSON
                  resolve:(RCTPromiseResolveBlock)resolve
                  reject:(RCTPromiseRejectBlock)reject)

RCT_EXTERN_METHOD(getTrafficStats:(RCTPromiseResolveBlock)resolve
                  reject:(RCTPromiseRejectBlock)reject)

@end
