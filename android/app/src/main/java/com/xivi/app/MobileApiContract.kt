package com.xivi.app

internal object MobileApiContract {
    const val VERSION = 1

    fun healthIsCompatible(status: String?, mobileApiVersion: Int): Boolean =
        status == "ok" && mobileApiVersion >= VERSION

    fun loginRequiresServerUpdate(isMobileLogin: Boolean, status: Int, errorCode: String?): Boolean =
        isMobileLogin && status == 401 && errorCode == "authentication_required"
}
